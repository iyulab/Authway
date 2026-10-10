package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenant"
)

// fixtureTenant creates a real tenant row — webhooks.tenant_id carries a
// foreign key to tenants(id).
func fixtureTenant(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := tenant.NewService(db).CreateTenant(tenant.CreateTenantRequest{
		Name: "webhook-test-" + suffix, Slug: "webhook-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
	return tn.ID
}

// setupPostgres mirrors the same-named helper already established across
// pkg/invitation, pkg/mfa, pkg/tenant, pkg/user, pkg/claims, and pkg/email.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres webhook tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func cleanupWebhook(t *testing.T, db *gorm.DB, id uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM webhook_deliveries WHERE webhook_id = ?`, id)
		db.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	})
}

// waitForDeliveries polls GetDeliveries until at least one row appears or the
// timeout elapses — Trigger dispatches delivery over a goroutine by design
// (fire-and-forget from the caller's perspective), so a test observing its
// effect must wait for it rather than assuming synchronous completion.
func waitForDeliveries(t *testing.T, svc Service, webhookID uuid.UUID, timeout time.Duration) []WebhookDelivery {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deliveries, err := svc.GetDeliveries(webhookID, 10)
		if err != nil {
			t.Fatalf("GetDeliveries: %v", err)
		}
		if len(deliveries) > 0 {
			return deliveries
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for a delivery to be recorded", timeout)
	return nil
}

// TestSignAndVerify guards the signature receivers check: it covers the time
// as well as the body, and a stale time fails even with the right secret.
func TestSignAndVerify(t *testing.T) {
	body := []byte(`{"type":"user.created"}`)
	secret := "test-secret"
	now := time.Unix(1_800_000_000, 0)
	header := Sign(secret, now.Unix(), body)

	if !Verify(header, body, secret, 5*time.Minute, now.Add(time.Minute)) {
		t.Fatal("expected a fresh signature to verify")
	}
	for name, ok := range map[string]bool{
		"tampered body": Verify(header, []byte(`{"type":"user.deleted"}`), secret, 5*time.Minute, now),
		"wrong secret":  Verify(header, body, "wrong-secret", 5*time.Minute, now),
		"stale":         Verify(header, body, secret, 5*time.Minute, now.Add(6*time.Minute)),
		"garbage":       Verify("t=1,v1=deadbeef", body, secret, 5*time.Minute, now),
		"time changed":  Verify(fmt.Sprintf("t=%d,%s", now.Unix()+1, strings.SplitN(header, ",", 2)[1]), body, secret, 5*time.Minute, now),
		"empty":         Verify("", body, secret, 5*time.Minute, now),
	} {
		if ok {
			t.Errorf("%s: expected Verify to fail", name)
		}
	}
}

func intp(n int) *int    { return &n }
func boolp(b bool) *bool { return &b }

// TestCreateWebhook_TakesDefaultsOnlyForWhatIsUnset guards the values a
// caller sends surviving the INSERT: GORM drops a zero value from the INSERT
// when the model field declares a default, which stored enabled=false and
// retry_count=0 as true and 3.
func TestCreateWebhook_TakesDefaultsOnlyForWhatIsUnset(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	cases := []struct {
		name        string
		req         CreateWebhookRequest
		wantEnabled bool
		wantRetry   int
		wantTimeout int
	}{
		{"unset values default", CreateWebhookRequest{}, true, DefaultRetryCount, DefaultTimeoutSecs},
		{"zero and false are kept", CreateWebhookRequest{Enabled: boolp(false), RetryCount: intp(0), TimeoutSecs: intp(1)}, false, 0, 1},
		{"in-range values pass through", CreateWebhookRequest{RetryCount: intp(5), TimeoutSecs: intp(60)}, true, 5, 60},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			req.Name, req.URL, req.Events = "test-"+tc.name, "https://example.com/hook", []string{"test"}
			wh, err := svc.Create(tenantID, &req)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			cleanupWebhook(t, db, wh.ID)

			stored, err := svc.GetByID(wh.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if stored.Enabled != tc.wantEnabled || stored.RetryCount != tc.wantRetry || stored.TimeoutSecs != tc.wantTimeout {
				t.Errorf("stored enabled=%v retry=%d timeout=%d, want %v %d %d",
					stored.Enabled, stored.RetryCount, stored.TimeoutSecs, tc.wantEnabled, tc.wantRetry, tc.wantTimeout)
			}
			if stored.Secret == "" {
				t.Error("expected a generated, non-empty secret")
			}
		})
	}
}

// TestCreateAndUpdate_RefuseInvalidValues guards that a value outside its
// range is refused with a message for the caller, never replaced or skipped.
func TestCreateAndUpdate_RefuseInvalidValues(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	valid := func() CreateWebhookRequest {
		return CreateWebhookRequest{Name: "valid", URL: "https://example.com/hook", Events: []string{"user.created"}}
	}
	bad := map[string]func(*CreateWebhookRequest){
		"blank name":       func(r *CreateWebhookRequest) { r.Name = " " },
		"relative url":     func(r *CreateWebhookRequest) { r.URL = "/hook" },
		"non-http url":     func(r *CreateWebhookRequest) { r.URL = "ftp://example.com/hook" },
		"no events":        func(r *CreateWebhookRequest) { r.Events = nil },
		"unknown event":    func(r *CreateWebhookRequest) { r.Events = []string{"user.exploded"} },
		"negative retries": func(r *CreateWebhookRequest) { r.RetryCount = intp(-1) },
		"too many retries": func(r *CreateWebhookRequest) { r.RetryCount = intp(MaxRetryCount + 1) },
		"zero timeout":     func(r *CreateWebhookRequest) { r.TimeoutSecs = intp(0) },
		"long timeout":     func(r *CreateWebhookRequest) { r.TimeoutSecs = intp(MaxTimeoutSecs + 1) },
	}
	for name, mutate := range bad {
		t.Run("create/"+name, func(t *testing.T) {
			req := valid()
			mutate(&req)
			wh, err := svc.Create(tenantID, &req)
			if err == nil {
				cleanupWebhook(t, db, wh.ID)
				t.Fatal("expected a refusal")
			}
			if apierror.Message(err, "") == "" {
				t.Fatalf("expected a refusal worded for the caller, got %v", err)
			}
		})
	}

	req := valid()
	wh, err := svc.Create(tenantID, &req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)
	newName := "renamed"
	if _, err := svc.Update(wh.ID, &UpdateWebhookRequest{Name: &newName, RetryCount: intp(999)}); apierror.Message(err, "") == "" {
		t.Fatalf("expected Update to refuse an out-of-range retry_count, got %v", err)
	}
	stored, _ := svc.GetByID(wh.ID)
	if stored.Name != "valid" {
		t.Errorf("a refused update must change nothing; name = %q", stored.Name)
	}
	if _, err := svc.Update(wh.ID, &UpdateWebhookRequest{Events: []string{"user.deleted", "*"}, Enabled: boolp(false)}); err != nil {
		t.Fatalf("Update events: %v", err)
	}
	stored, _ = svc.GetByID(wh.ID)
	if len(stored.Events) != 2 || stored.Enabled {
		t.Errorf("after update events=%v enabled=%v", stored.Events, stored.Enabled)
	}
}

// TestGetByIDAndListByTenant_ExcludeSoftDeleted guards the manual
// deleted_at-IS-NULL filter Delete/GetByID/ListByTenant all rely on — Webhook
// uses a plain *time.Time, not gorm.DeletedAt, so GORM applies no automatic
// scope here; every read path has to filter it explicitly and correctly.
func TestGetByIDAndListByTenant_ExcludeSoftDeleted(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	wh, err := svc.Create(tenantID, &CreateWebhookRequest{Name: "to-delete", URL: "https://example.com/hook", Events: []string{"test"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)

	if err := svc.Delete(wh.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := svc.GetByID(wh.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a soft-deleted webhook, got %v", err)
	}
	if err := svc.Delete(wh.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleting it again to report ErrNotFound, got %v", err)
	}

	list, err := svc.ListByTenant(tenantID)
	if err != nil {
		t.Fatalf("ListByTenant: %v", err)
	}
	for _, w := range list {
		if w.ID == wh.ID {
			t.Fatal("expected ListByTenant to exclude the soft-deleted webhook")
		}
	}
}

// TestTrigger_DeliversOnlyToSubscribedEnabledWebhooksAndSignsThePayload
// exercises the real dispatch path end-to-end: event-type subscription
// filtering, HMAC signing of the exact bytes the receiver gets, and the
// delivery record left behind for the caller to audit.
func TestTrigger_DeliversOnlyToSubscribedEnabledWebhooksAndSignsThePayload(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	var receivedBody []byte
	var receivedSig string
	received := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedBody = body
		receivedSig = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(http.StatusOK)
		select {
		case received <- struct{}{}:
		default:
		}
	}))
	defer ts.Close()

	subscribed, err := svc.Create(tenantID, &CreateWebhookRequest{
		Name: "subscribed", URL: ts.URL, Events: []string{string(EventUserCreated)}, RetryCount: intp(0), TimeoutSecs: intp(5),
	})
	if err != nil {
		t.Fatalf("Create (subscribed): %v", err)
	}
	cleanupWebhook(t, db, subscribed.ID)

	notSubscribed, err := svc.Create(tenantID, &CreateWebhookRequest{
		Name: "not-subscribed", URL: ts.URL, Events: []string{string(EventUserDeleted)}, RetryCount: intp(0), TimeoutSecs: intp(5),
	})
	if err != nil {
		t.Fatalf("Create (not-subscribed): %v", err)
	}
	cleanupWebhook(t, db, notSubscribed.ID)

	if err := svc.Trigger(tenantID, EventUserCreated, map[string]string{"email": "user@example.com"}); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	deliveries := waitForDeliveries(t, svc, subscribed.ID, 3*time.Second)
	if len(deliveries) != 1 {
		t.Fatalf("expected exactly 1 delivery for the subscribed webhook, got %d", len(deliveries))
	}
	if !deliveries[0].Success || deliveries[0].StatusCode != http.StatusOK {
		t.Fatalf("expected a successful 200 delivery, got success=%v status=%d", deliveries[0].Success, deliveries[0].StatusCode)
	}

	notSubscribedDeliveries, err := svc.GetDeliveries(notSubscribed.ID, 10)
	if err != nil {
		t.Fatalf("GetDeliveries (not-subscribed): %v", err)
	}
	if len(notSubscribedDeliveries) != 0 {
		t.Fatalf("expected 0 deliveries for a webhook not subscribed to this event, got %d", len(notSubscribedDeliveries))
	}

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("expected the test HTTP server to have received the request by now")
	}
	if !Verify(receivedSig, receivedBody, subscribed.Secret, time.Minute, time.Now()) {
		t.Fatal("expected the X-Webhook-Signature header to verify against the webhook's own secret and the exact received body")
	}
	var payload WebhookPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("decode received payload: %v", err)
	}
	if payload.Type != EventUserCreated || payload.TenantID != tenantID.String() {
		t.Fatalf("unexpected payload contents: %+v", payload)
	}
}

// TestTrigger_RecordsFailedDeliveryOnServerError guards that a non-2xx
// response is recorded as a failed delivery with the status captured, not
// silently dropped.
func TestTrigger_RecordsFailedDeliveryOnServerError(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	wh, err := svc.Create(tenantID, &CreateWebhookRequest{
		Name: "failing", URL: ts.URL, Events: []string{string(EventTypeTest)}, RetryCount: intp(0), TimeoutSecs: intp(5),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)

	if err := svc.Trigger(tenantID, EventTypeTest, nil); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	deliveries := waitForDeliveries(t, svc, wh.ID, 5*time.Second)
	if deliveries[0].Success {
		t.Fatal("expected the delivery to be recorded as unsuccessful")
	}
	if deliveries[0].StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status_code=500 recorded, got %d", deliveries[0].StatusCode)
	}
}

// TestTest_DeliversOnceToThatWebhookAndReportsTheOutcome guards the test
// endpoint's contract: it reaches this webhook even when it is disabled and
// not subscribed to the test event, makes exactly one attempt, and returns
// the delivery whether the receiver accepted it or not.
func TestTest_DeliversOnceToThatWebhookAndReportsTheOutcome(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	hits := 0
	status := http.StatusNoContent
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(status)
	}))
	defer ts.Close()

	wh, err := svc.Create(tenantID, &CreateWebhookRequest{
		Name: "disabled", URL: ts.URL, Events: []string{string(EventUserCreated)}, Enabled: boolp(false), RetryCount: intp(3), TimeoutSecs: intp(5),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)

	d, err := svc.Test(wh.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !d.Success || d.StatusCode != http.StatusNoContent || hits != 1 {
		t.Fatalf("success=%v status=%d hits=%d, want true 204 1", d.Success, d.StatusCode, hits)
	}
	if d.ID == uuid.Nil {
		t.Fatal("expected the returned delivery to carry the id it was recorded under")
	}
	recorded, err := svc.GetDeliveries(wh.ID, 10)
	if err != nil || len(recorded) != 1 || recorded[0].ID != d.ID {
		t.Fatalf("expected the delivery to be recorded once under id %s, got %+v (err %v)", d.ID, recorded, err)
	}

	status = http.StatusBadGateway
	d, err = svc.Test(wh.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if d.Success || d.StatusCode != http.StatusBadGateway || hits != 2 {
		t.Fatalf("success=%v status=%d hits=%d, want false 502 2 (one attempt, no retries)", d.Success, d.StatusCode, hits)
	}

	if _, err := svc.Test(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown webhook, got %v", err)
	}
}

// TestRotateSecret_SignsWithTheNewSecretOnly guards rotation: deliveries after
// it verify under the new secret and not the old one.
func TestRotateSecret_SignsWithTheNewSecretOnly(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)

	var sig string
	var body []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig = r.Header.Get("X-Webhook-Signature")
		body, _ = io.ReadAll(r.Body)
	}))
	defer ts.Close()

	wh, err := svc.Create(tenantID, &CreateWebhookRequest{Name: "rotate", URL: ts.URL, Events: []string{"test"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)

	secret, err := svc.RotateSecret(wh.ID)
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if secret == "" || secret == wh.Secret {
		t.Fatal("expected a new, non-empty secret")
	}
	if _, err := svc.Test(wh.ID); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !Verify(sig, body, secret, time.Minute, time.Now()) {
		t.Fatal("expected the delivery to verify under the new secret")
	}
	if Verify(sig, body, wh.Secret, time.Minute, time.Now()) {
		t.Fatal("expected the old secret to stop verifying")
	}
	if _, err := svc.RotateSecret(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown webhook, got %v", err)
	}
}

// TestFromAudit_AnAuditEntryReachesTheSubscribedReceiver guards the whole
// chain the admin sees as "webhooks": recording a lifecycle event in the
// audit log delivers it, signed, to a webhook subscribed to that event. Until
// this was wired, every event but `test` was offered and none was ever sent.
func TestFromAudit_AnAuditEntryReachesTheSubscribedReceiver(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), AllowPrivateTargets(true))
	tenantID := fixtureTenant(t, db)
	audits := audit.NewService(db, zap.NewNop())
	audits.Subscribe(FromAudit(svc, zap.NewNop()))

	bodies := make(chan []byte, 4)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	wh, err := svc.Create(tenantID, &CreateWebhookRequest{
		Name: "deletions", URL: ts.URL, Events: []string{string(EventUserDeleted)}, RetryCount: intp(0), TimeoutSecs: intp(5),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)
	t.Cleanup(func() { db.Exec(`DELETE FROM audit_logs WHERE tenant_id = ?`, tenantID) })

	userID, adminID := uuid.New(), uuid.New()
	record := func(action audit.AuditAction, success bool) {
		t.Helper()
		if err := audits.Log(context.Background(), &audit.AuditEntry{
			TenantID: tenantID, Action: action, Success: success,
			ResourceType: "user", ResourceID: userID.String(), ActorType: "admin", ActorID: &adminID,
		}); err != nil {
			t.Fatalf("Log %s: %v", action, err)
		}
	}
	record(audit.ActionUserLogin, true)    // not subscribed to
	record(audit.ActionUserDeleted, false) // a failed operation is not an event
	record(audit.ActionUserDeleted, true)

	var body []byte
	select {
	case body = <-bodies:
	case <-time.After(3 * time.Second):
		t.Fatal("the receiver got nothing for a recorded user.deleted")
	}
	var payload struct {
		Type     EventType `json:"type"`
		TenantID string    `json:"tenant_id"`
		Data     EventData `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	want := EventData{Resource: EventRef{Type: "user", ID: userID.String()}, Actor: EventRef{Type: "admin", ID: adminID.String()}}
	if payload.Type != EventUserDeleted || payload.TenantID != tenantID.String() || payload.Data != want {
		t.Errorf("received %s, want user.deleted for tenant %s with data %+v", body, tenantID, want)
	}

	select {
	case extra := <-bodies:
		t.Errorf("the receiver also got %s; only the successful user.deleted is an event it subscribed to", extra)
	case <-time.After(300 * time.Millisecond):
	}
}
