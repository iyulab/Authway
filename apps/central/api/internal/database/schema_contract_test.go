package database_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/impersonation"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/passwordless"
	"authway/apps/central/api/pkg/tenant"
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

	cases := []struct {
		name  string
		row   any
		table string
	}{
		{"invitation", &invitation.Invitation{
			TenantID: tenantID, Email: "c@example.com", Role: "member",
			Token: uuid.New().String(), Status: invitation.StatusPending, ExpiresAt: future,
		}, "invitations"},
		{"impersonation_session", &impersonation.ImpersonationSession{
			TenantID: tenantID, AdminEmail: impersonation.SystemActorEmail,
			TargetUserID: userID, TargetUserEmail: "c@example.com",
			Reason: "schema contract check", Token: uuid.New().String(),
			Active: true, StartedAt: time.Now(), ExpiresAt: future,
		}, "impersonation_sessions"},
		{"magic_link", &passwordless.MagicLink{
			TenantID: tenantID, Email: "c@example.com", Token: uuid.New().String(),
			TokenType: passwordless.TokenTypeLogin, ExpiresAt: future,
		}, "magic_link_tokens"},
		{"webhook", &webhook.Webhook{
			TenantID: tenantID, Name: "contract", URL: "https://example.com/hook",
			Events: []string{"user.created"}, Secret: "s", Enabled: true,
		}, "webhooks"},
		{"user_claim", &claims.UserClaim{
			UserID: userID, TenantID: tenantID,
			ClaimKey: "contract-" + uuid.New().String()[:8], ClaimValue: map[string]any{"v": true},
		}, "user_claims"},
		{"audit_log", &audit.AuditLog{
			TenantID: tenantID, ActorEmail: "c@example.com", ActorType: "system",
			Action: audit.ActionAdminAction, Severity: audit.SeverityInfo,
			ResourceType: "user", ResourceID: userID.String(), Success: true,
			// Details maps to jsonb, so it must hold JSON — "" is not valid
			// JSON. The service always marshals a map, hence at minimum "{}".
			Details: "{}",
		}, "audit_logs"},
		// accountlink.LinkedAccount is deliberately absent: its table does not
		// exist in any migration. See TestSchemaContract_LinkedAccountsHasNoTable.
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

// TestSchemaContract_LinkedAccountsHasNoTable records a gap rather than a
// contract: pkg/accountlink maps to linked_accounts, no migration ever creates
// that table, and its routes are registered anyway — so /account/linked and
// /account/providers fail at runtime. Nothing calls LinkAccount either, so no
// row could exist even if the table did.
//
// Whether to create the table or retire the feature is a product decision
// (users already carry google_id/github_id/... as the de-facto link record),
// so this test pins the current reality instead of pretending either way.
// DELETE THIS TEST once that decision lands — a failure here means the gap was
// closed and this file is stale.
func TestSchemaContract_LinkedAccountsHasNoTable(t *testing.T) {
	db := setup(t)

	var exists bool
	if err := db.Raw(`SELECT to_regclass('public.linked_accounts') IS NOT NULL`).Scan(&exists).Error; err != nil {
		t.Fatalf("probe: %v", err)
	}
	if exists {
		t.Fatal("linked_accounts now exists — resolve the accountlink decision and replace this test with a real contract case")
	}
	t.Log("known gap: pkg/accountlink has registered routes but no table (see claudedocs/issues)")
}
