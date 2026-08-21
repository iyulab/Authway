package webhook

import (
	"encoding/json"
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

// TestCreateWebhook_AppliesDefaultsAndClampsOutOfRangeValues guards Create's
// silent-clamp behavior for RetryCount/TimeoutSecs — zero and out-of-range
// values both fall back to the default rather than being stored verbatim.
func TestCreateWebhook_AppliesDefaultsAndClampsOutOfRangeValues(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop())
	tenantID := fixtureTenant(t, db)

	cases := []struct {
		name           string
		retryCount     int
		timeoutSecs    int
		wantRetryCount int
		wantTimeout    int
	}{
		{"zero values default", 0, 0, 3, 30},
		{"out-of-range values default", 999, 999, 3, 30},
		{"in-range values pass through", 5, 60, 5, 60},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wh, err := svc.Create(tenantID, &CreateWebhookRequest{
				Name: "test-" + tc.name, URL: "https://example.com/hook",
				Events: []string{"test"}, RetryCount: tc.retryCount, TimeoutSecs: tc.timeoutSecs,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			cleanupWebhook(t, db, wh.ID)

			if wh.RetryCount != tc.wantRetryCount {
				t.Errorf("RetryCount = %d, want %d", wh.RetryCount, tc.wantRetryCount)
			}
			if wh.TimeoutSecs != tc.wantTimeout {
				t.Errorf("TimeoutSecs = %d, want %d", wh.TimeoutSecs, tc.wantTimeout)
			}
			if wh.Secret == "" {
				t.Error("expected a generated, non-empty secret")
			}
		})
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

	if _, err := svc.GetByID(wh.ID); err == nil {
		t.Fatal("expected GetByID to fail for a soft-deleted webhook")
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

// TestUpdate_IgnoresOutOfRangeRetryAndTimeoutButKeepsOtherFields guards a
// behavior that differs from Create: Update silently SKIPS an out-of-range
// RetryCount/TimeoutSecs (leaving the existing stored value untouched)
// rather than clamping to a default — worth pinning explicitly since a
// caller could otherwise reasonably assume Update clamps the same way
// Create does.
func TestUpdate_IgnoresOutOfRangeRetryAndTimeoutButKeepsOtherFields(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop())
	tenantID := fixtureTenant(t, db)

	wh, err := svc.Create(tenantID, &CreateWebhookRequest{Name: "original", URL: "https://example.com/hook", Events: []string{"test"}, RetryCount: 5, TimeoutSecs: 60})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupWebhook(t, db, wh.ID)

	newName := "renamed"
	outOfRangeRetry := 999
	updated, err := svc.Update(wh.ID, &UpdateWebhookRequest{Name: &newName, RetryCount: &outOfRangeRetry})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Name = %q, want %q", updated.Name, newName)
	}
	if updated.RetryCount != 5 {
		t.Errorf("expected RetryCount to remain 5 when the update value is out of range, got %d", updated.RetryCount)
	}
	if updated.TimeoutSecs != 60 {
		t.Errorf("expected TimeoutSecs to remain untouched (not provided in this update), got %d", updated.TimeoutSecs)
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
		Name: "subscribed", URL: ts.URL, Events: []string{string(EventUserCreated)}, RetryCount: 1, TimeoutSecs: 5,
	})
	if err != nil {
		t.Fatalf("Create (subscribed): %v", err)
	}
	cleanupWebhook(t, db, subscribed.ID)

	notSubscribed, err := svc.Create(tenantID, &CreateWebhookRequest{
		Name: "not-subscribed", URL: ts.URL, Events: []string{string(EventUserDeleted)}, RetryCount: 1, TimeoutSecs: 5,
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
		Name: "failing", URL: ts.URL, Events: []string{string(EventTypeTest)}, RetryCount: 1, TimeoutSecs: 5,
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
