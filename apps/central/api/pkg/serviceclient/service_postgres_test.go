package serviceclient

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/tenant"
)

// setupPostgres mirrors the same-named helper already established in
// pkg/invitation, pkg/mfa, pkg/tenant, pkg/user, pkg/claims, pkg/email,
// pkg/webhook.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres serviceclient tests")
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

func fixtureTenant(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := tenant.NewService(db).CreateTenant(tenant.CreateTenantRequest{
		Name: "svcclient-test-" + suffix, Slug: "svcclient-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
	return tn.ID
}

// testHydraAdminURL points at the local Hydra admin API used by this
// package's live verification, the same instance every other
// _postgres_test.go in this codebase runs its live checks against.
func testHydraAdminURL() string {
	if v := os.Getenv("HYDRA_ADMIN_URL"); v != "" {
		return v
	}
	return "http://localhost:4445"
}

func TestCreate_RejectsUnknownScope(t *testing.T) {
	db := setupPostgres(t)
	tenantID := fixtureTenant(t, db)
	svc := NewService(db, zap.NewNop(), hydra.NewClient(testHydraAdminURL()))

	_, _, err := svc.Create(tenantID, &CreateServiceClientRequest{
		Name: "bad-scope", Scopes: []string{"admin.users:write"},
	})
	if err == nil {
		t.Fatal("expected an error for an unallowlisted scope, got nil")
	}
}

func TestCreate_RegistersInHydraAndDB(t *testing.T) {
	db := setupPostgres(t)
	tenantID := fixtureTenant(t, db)
	hydraClient := hydra.NewClient(testHydraAdminURL())
	svc := NewService(db, zap.NewNop(), hydraClient)

	sc, creds, err := svc.Create(tenantID, &CreateServiceClientRequest{
		Name: "scoped-service-provisioning", Scopes: []string{"admin.clients:write"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		hydraClient.DeleteOAuth2Client(sc.HydraClientID)
		db.Exec(`DELETE FROM service_clients WHERE id = ?`, sc.ID)
	})

	if creds.ClientID == "" || creds.ClientSecret == "" {
		t.Fatalf("expected non-empty credentials, got %+v", creds)
	}
	if sc.TenantID != tenantID {
		t.Fatalf("TenantID = %v, want %v", sc.TenantID, tenantID)
	}
	if sc.IsRevoked() {
		t.Fatal("freshly created service client must not be revoked")
	}
	if !sc.HasScope("admin.clients:write") {
		t.Fatalf("expected granted_scopes to include admin.clients:write, got %v", sc.GrantedScopes)
	}

	fetched, err := svc.GetByHydraClientID(creds.ClientID)
	if err != nil {
		t.Fatalf("GetByHydraClientID: %v", err)
	}
	if fetched.ID != sc.ID {
		t.Fatalf("GetByHydraClientID returned a different row: got %v want %v", fetched.ID, sc.ID)
	}
}

func TestRevoke_SetsRevokedAtAndDeletesHydraClient(t *testing.T) {
	db := setupPostgres(t)
	tenantID := fixtureTenant(t, db)
	hydraClient := hydra.NewClient(testHydraAdminURL())
	svc := NewService(db, zap.NewNop(), hydraClient)

	sc, creds, err := svc.Create(tenantID, &CreateServiceClientRequest{
		Name: "revoke-me", Scopes: []string{"admin.clients:write"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM service_clients WHERE id = ?`, sc.ID) })

	if err := svc.Revoke(sc.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	fetched, err := svc.GetByHydraClientID(creds.ClientID)
	if err != nil {
		t.Fatalf("GetByHydraClientID after revoke: %v", err)
	}
	if !fetched.IsRevoked() {
		t.Fatal("expected revoked_at to be set after Revoke")
	}

	if _, err := hydraClient.GetOAuth2Client(creds.ClientID); err == nil {
		t.Fatal("expected the Hydra client to be deleted after Revoke, but it still exists")
	}
}
