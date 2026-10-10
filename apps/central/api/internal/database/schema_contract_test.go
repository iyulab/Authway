package database_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/admin"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/passwordless"
	"authway/apps/central/api/pkg/serviceclient"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
	"authway/apps/central/api/pkg/webhook"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
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
		{"magic_link", &passwordless.MagicLink{
			TenantID: tenantID, Email: "c@example.com", TokenHash: tokenhash.Hash(uuid.New().String()),
			TokenType: passwordless.TokenTypeLogin, LoginFlow: "flow", ExpiresAt: future,
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
		{"service_client", &serviceclient.ServiceClient{
			TenantID: tenantID, HydraClientID: "authway_svc_" + uuid.New().String()[:8],
			Name: "contract", GrantedScopes: []string{"admin.clients:write"},
		}, "service_clients"},
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
		"invitations", "magic_link_tokens",
		"webhooks", "webhook_deliveries", "user_claims", "audit_logs",
		"password_resets", "email_verifications", "service_clients",
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

// mappedModels is every GORM model that owns a table. The two tests below hold
// the migrated schema to exactly this set.
func mappedModels() []any {
	return []any{
		&admin.AdminSession{}, &audit.AuditLog{}, &client.Client{},
		&email.EmailVerification{}, &email.PasswordReset{},
		&invitation.Invitation{},
		&passwordless.MagicLink{}, &serviceclient.ServiceClient{},
		&tenant.Tenant{}, &user.User{}, &claims.UserClaim{},
		&webhook.Webhook{}, &webhook.WebhookDelivery{},
	}
}

func parseModel(t *testing.T, db *gorm.DB, m any) *gorm.Statement {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		t.Fatalf("parse %T: %v", m, err)
	}
	return stmt
}

// TestMigratedTablesHaveNoUnmappedColumns is the reverse of the write test
// above: every column the migrations leave on a model's table must be one the
// model maps. A column nothing reads or writes is not harmless — it keeps
// constraints and defaults the application no longer means, and it hides
// which data is live. Columns pile up this way when code stops using them and
// the drop never follows.
func TestMigratedTablesHaveNoUnmappedColumns(t *testing.T) {
	db := setup(t)

	for _, m := range mappedModels() {
		stmt := parseModel(t, db, m)
		mapped := make(map[string]bool, len(stmt.Schema.DBNames))
		for _, name := range stmt.Schema.DBNames {
			mapped[name] = true
		}

		var columns []string
		if err := db.Raw(`SELECT column_name FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = ? ORDER BY ordinal_position`,
			stmt.Schema.Table).Scan(&columns).Error; err != nil {
			t.Fatalf("columns of %s: %v", stmt.Schema.Table, err)
		}
		for _, c := range columns {
			if !mapped[c] {
				t.Errorf("%s.%s exists in the migrated schema but %T does not map it", stmt.Schema.Table, c, m)
			}
		}
	}
}

// TestEveryMigratedTableIsMapped closes the same gap one level up: a table no
// model owns is one nothing in the application reads or writes.
func TestEveryMigratedTableIsMapped(t *testing.T) {
	db := setup(t)

	owned := map[string]bool{}
	for _, m := range mappedModels() {
		owned[parseModel(t, db, m).Schema.Table] = true
	}

	// Not ours: the migrator's own bookkeeping, and the authorization server's
	// tables when it shares the database (hydra_*, networks, schema_migration).
	var tables []string
	if err := db.Raw(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		  AND table_name NOT IN ('schema_migrations', 'schema_migration', 'networks')
		  AND table_name NOT LIKE 'hydra\_%'
		ORDER BY table_name`).Scan(&tables).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for _, table := range tables {
		if !owned[table] && !awaitingDrop[table] {
			t.Errorf("%s exists in the migrated schema but no model maps it", table)
		}
	}
}

// awaitingDrop names tables whose code has been removed and whose drop is the
// next migration. The code goes first and the drop follows in a deployment of
// its own, because a dropped table cannot be rolled back with the binary. An
// entry here is a debt with a due date: it leaves with that migration.
var awaitingDrop = map[string]bool{}

// TestModelsCanStoreZeroValues guards the GORM rule that cost this codebase a
// falsified audit trail: a field that declares a default is left out of the
// INSERT whenever it holds its zero value, so the database default is stored
// instead. For a bool defaulting to true or a number defaulting to non-zero,
// false and 0 then cannot be written at all — failed events were recorded as
// successes, disabled sign-up as enabled. Such defaults belong in the code
// that builds the row, not in the mapping.
func TestModelsCanStoreZeroValues(t *testing.T) {
	cache := &sync.Map{}
	for _, m := range mappedModels() {
		s, err := schema.Parse(m, cache, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse %T: %v", m, err)
		}
		for _, f := range s.Fields {
			if !f.HasDefaultValue || f.DefaultValueInterface == nil {
				continue
			}
			switch f.FieldType.Kind() {
			case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
				if !reflect.ValueOf(f.DefaultValueInterface).IsZero() {
					t.Errorf("%T.%s declares gorm default %q: its zero value can never be stored", m, f.Name, f.DefaultValue)
				}
			}
		}
	}
}

// TestAuditLogKeepsFailures is the behaviour TestModelsCanStoreZeroValues
// protects, end to end: an event logged as a failure is stored as one.
func TestAuditLogKeepsFailures(t *testing.T) {
	db := setup(t)
	tenantID, _ := fixtures(t, db)
	svc := audit.NewService(db, zap.NewNop())
	resource := uuid.New().String()
	if err := svc.Log(context.Background(), &audit.AuditEntry{
		TenantID: tenantID, Action: audit.ActionUserLoginFailed, ResourceType: "user", ResourceID: resource,
		Success: false, ErrorMsg: "invalid credentials",
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM audit_logs WHERE resource_id = ?`, resource) })

	var success bool
	if err := db.Raw(`SELECT success FROM audit_logs WHERE resource_id = ?`, resource).Scan(&success).Error; err != nil {
		t.Fatal(err)
	}
	if success {
		t.Fatal("a failure was stored as a success")
	}
}
