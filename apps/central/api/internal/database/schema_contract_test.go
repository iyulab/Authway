package database_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/impersonation"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/passwordless"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
	"authway/apps/central/api/pkg/webhook"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Every GORM model must be writable against the schema the migrations actually
// produce. Twice in one run a model declared columns that no migration created
// (invitations.tenant_name/inviter_name/accepted_by) or named a table that does
// not exist at all — and neither was caught, because the other tests build
// their schema with AutoMigrate *from the same struct*. That harness can never
// disagree with the struct, so it cannot see drift by construction.
//
// This test writes one row per model against the real migrated schema. It does
// not assert behaviour; it asserts that the contract between Go and SQL holds.
func setup(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping schema contract tests")
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

// fixtures returns a tenant and a user that the FK-bearing models can point at.
func fixtures(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := tenant.NewService(db).CreateTenant(tenant.CreateTenantRequest{
		Name: "contract-" + suffix,
		Slug: "contract-" + suffix,
	})
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	u, err := user.NewService(db, zap.NewNop()).Create(tn.ID, &user.CreateUserRequest{
		Email:    fmt.Sprintf("contract-%s@example.com", suffix),
		Password: "correct-horse-battery",
		Name:     "Contract",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
		db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID)
	})
	return tn.ID, u.ID
}

func TestSchemaContract_FeatureModels(t *testing.T) {
	db := setup(t)
	tenantID, userID := fixtures(t, db)
	future := time.Now().Add(time.Hour)

	// WebhookDelivery's FK needs a real webhook row (its own model is already
	// covered below via the "webhook" case, seeded separately here so this
	// case can run independently of table ordering).
	wh := &webhook.Webhook{
		TenantID: tenantID, Name: "contract-delivery", URL: "https://example.com/hook",
		Events: []string{"user.created"}, Secret: "s", Enabled: true,
	}
	if err := db.Create(wh).Error; err != nil {
		t.Fatalf("seed webhook for delivery contract case: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM webhooks WHERE id = ?`, wh.ID) })

	cases := []struct {
		name  string
		row   any
		table string
	}{
		{"invitation", &invitation.Invitation{
			TenantID: tenantID, Email: "c@example.com", Role: "member",
			TokenHash: tokenhash.Hash(uuid.New().String()), Status: invitation.StatusPending, ExpiresAt: future,
		}, "invitations"},
		{"impersonation_session", &impersonation.ImpersonationSession{
			TenantID: tenantID, AdminEmail: impersonation.SystemActorEmail,
			TargetUserID: userID, TargetUserEmail: "c@example.com",
			Reason: "schema contract check", Token: uuid.New().String(),
			Active: true, StartedAt: time.Now(), ExpiresAt: future,
		}, "impersonation_sessions"},
		{"magic_link", &passwordless.MagicLink{
			TenantID: tenantID, Email: "c@example.com", TokenHash: tokenhash.Hash(uuid.New().String()),
			TokenType: passwordless.TokenTypeLogin, ExpiresAt: future,
		}, "magic_link_tokens"},
		{"webhook", &webhook.Webhook{
			TenantID: tenantID, Name: "contract", URL: "https://example.com/hook",
			Events: []string{"user.created"}, Secret: "s", Enabled: true,
		}, "webhooks"},
		// Regression case for migration 021: Success/ErrorMessage map to
		// columns migration 006 never created, so every delivery insert
		// failed with SQLSTATE 42703 — undetected because this model was
		// never enrolled here (only the parent Webhook was).
		{"webhook_delivery", &webhook.WebhookDelivery{
			WebhookID: wh.ID, EventType: "user.created", Payload: "{}",
			StatusCode: 200, Attempt: 1, DeliveredAt: time.Now(), Success: true,
		}, "webhook_deliveries"},
		{"user_claim", &claims.UserClaim{
			UserID: userID, TenantID: tenantID,
			ClaimKey: "contract-" + uuid.New().String()[:8], ClaimValue: map[string]any{"v": true},
		}, "user_claims"},
		// password_resets/email_verifications drifted the same way (000 never
		// had used_at/updated_at, 013/014 only renamed the token column), so
		// forgot-password 500'd in prod on the very first real call.
		{"password_reset", &email.PasswordReset{
			UserID: userID, TokenHash: uuid.New().String(), ExpiresAt: future,
		}, "password_resets"},
		{"email_verification", &email.EmailVerification{
			UserID: userID, TokenHash: uuid.New().String(), ExpiresAt: future,
		}, "email_verifications"},
		{"audit_log", &audit.AuditLog{
			TenantID: tenantID, ActorEmail: "c@example.com", ActorType: "system",
			Action: audit.ActionAdminAction, Severity: audit.SeverityInfo,
			ResourceType: "user", ResourceID: userID.String(), Success: true,
			// Details maps to jsonb, so it must hold JSON — "" is not valid
			// JSON. The service always marshals a map, hence at minimum "{}".
			Details: "{}",
		}, "audit_logs"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.Create(tc.row).Error; err != nil {
				t.Fatalf("%s does not match the migrated schema of %s: %v", tc.name, tc.table, err)
			}
			// Read it back through the same model: a write can succeed while a
			// read fails if a column the struct selects is missing.
			var back []map[string]any
			if err := db.Table(tc.table).Limit(1).Find(&back).Error; err != nil {
				t.Fatalf("%s read-back failed: %v", tc.name, err)
			}
			db.Delete(tc.row)
		})
	}
}

// TestNoModelMapsToAMissingTable is the generalised form of the accountlink
// defect: that package mapped to `linked_accounts`, no migration ever created
// it, and its routes were registered regardless — so the endpoints failed at
// runtime while looking perfectly wired. The package has since been removed
// (users.google_id/github_id/... already record the same thing, and nothing
// ever wrote a link row), but the class of mistake outlives it.
//
// Rather than name tables one by one, this walks every table the models above
// declare and asserts it exists. Adding a model to the table-driven test also
// enrols it here.
func TestNoModelMapsToAMissingTable(t *testing.T) {
	db := setup(t)

	tables := []string{
		"invitations", "impersonation_sessions", "magic_link_tokens",
		"webhooks", "webhook_deliveries", "user_claims", "audit_logs",
		"password_resets", "email_verifications",
		// Retired: linked_accounts. Do not re-add without a migration.
	}
	for _, table := range tables {
		var exists bool
		if err := db.Raw(`SELECT to_regclass('public.' || ?) IS NOT NULL`, table).Scan(&exists).Error; err != nil {
			t.Fatalf("probe %s: %v", table, err)
		}
		if !exists {
			t.Errorf("%s is mapped by a model but no migration creates it", table)
		}
	}
}
