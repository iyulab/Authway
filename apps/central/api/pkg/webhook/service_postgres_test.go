package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/apierror"
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

// TestSignAndVerifySignature guards the HMAC pairing itself — Trigger's
// receivers authenticate deliveries by recomputing this, so a break here
// silently defeats every consumer's signature check.
func TestSignAndVerifySignature(t *testing.T) {
	payload := []byte(`{"type":"user.created"}`)
	secret := "test-secret"

	sig := SignPayload(payload, secret)
	if !VerifySignature(payload, sig, secret) {
		t.Fatal("expected VerifySignature to accept a signature it just produced")
	}
	if VerifySignature([]byte(`{"type":"user.deleted"}`), sig, secret) {
		t.Fatal("expected VerifySignature to reject a tampered payload")
	}
	if VerifySignature(payload, sig, "wrong-secret") {
		t.Fatal("expected VerifySignature to reject the wrong secret")
	}
	if VerifySignature(payload, "deadbeef", secret) {
		t.Fatal("expected VerifySignature to reject a garbage signature")
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
	svc := NewService(db, zap.NewNop())
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
	svc := NewService(db, zap.NewNop())
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
	svc := NewService(db, zap.NewNop())
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
	svc := NewService(db, zap.NewNop())
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
	if !VerifySignature(receivedBody, receivedSig, subscribed.Secret) {
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
	svc := NewService(db, zap.NewNop())
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
	svc := NewService(db, zap.NewNop())
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
