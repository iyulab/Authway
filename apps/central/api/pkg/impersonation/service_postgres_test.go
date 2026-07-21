package impersonation

import (
	"fmt"
	"os"
	"testing"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Impersonation carried the same defect as invitations: the handler attributed
// admin-API-key requests to a hard-coded UUID with no users row, while
// admin_id was NOT NULL REFERENCES users(id) — so the admin-key path could
// never create a session. Migration 017 makes the column nullable; these tests
// run against real Postgres because the constraint is the thing under test.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres impersonation tests")
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

// seed returns a service plus a tenant holding one target user.
func seed(t *testing.T) (Service, *gorm.DB, *tenant.Tenant, *user.User) {
	t.Helper()
	db := setupPostgres(t)
	tenantService := tenant.NewService(db)
	userService := user.NewService(db, zap.NewNop())
	auditService := audit.NewService(db, zap.NewNop())

	suffix := uuid.New().String()[:8]
	tn, err := tenantService.CreateTenant(tenant.CreateTenantRequest{
		Name: "imp-test-" + suffix,
		Slug: "imp-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	target, err := userService.Create(tn.ID, &user.CreateUserRequest{
		Email:    fmt.Sprintf("target-%s@example.com", suffix),
		Password: "correct-horse-battery",
		Name:     "Target",
	})
	if err != nil {
		t.Fatalf("seed target: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM impersonation_sessions WHERE target_user_id = ?`, target.ID)
		db.Exec(`DELETE FROM users WHERE id = ?`, target.ID)
	})

	return NewService(db, userService, auditService, zap.NewNop()), db, tn, target
}

// TestStartImpersonation_SystemActor_Succeeds is the regression guard for the
// FK deadlock: an admin-API-key caller has no user row, and that must be
// expressible rather than rejected.
func TestStartImpersonation_SystemActor_Succeeds(t *testing.T) {
	svc, db, tn, target := seed(t)

	resp, err := svc.StartImpersonation(tn.ID, nil, &StartImpersonationRequest{
		TargetUserID: target.ID,
		Reason:       "regression guard for the system-actor path",
	}, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("system-actor impersonation must succeed, got: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected a session token")
	}

	// Assert on the columns, not the struct: NOT NULL was the actual blocker.
	var isNull bool
	var email string
	if err := db.Raw(
		`SELECT admin_id IS NULL, admin_email FROM impersonation_sessions WHERE target_user_id = ?`,
		target.ID,
	).Row().Scan(&isNull, &email); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !isNull {
		t.Error("admin_id must be NULL for a system-actor session")
	}
	if email != SystemActorEmail {
		t.Errorf("admin_email = %q, want %q — the audit trail must still name an actor", email, SystemActorEmail)
	}
}

// TestValidateToken_SystemActorSession_DoesNotPanic covers the read path. The
// first cut of this change wrote the session row and then died rendering it:
// uuid.UUID is an array type, so String() on a nil *uuid.UUID panics, which
// surfaced as a 500 after the write had already happened.
func TestValidateToken_SystemActorSession_DoesNotPanic(t *testing.T) {
	svc, _, tn, target := seed(t)

	resp, err := svc.StartImpersonation(tn.ID, nil, &StartImpersonationRequest{
		TargetUserID: target.ID,
		Reason:       "regression guard for nil-admin rendering",
	}, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	session, err := svc.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if session.AdminID != nil {
		t.Errorf("AdminID = %v, want nil", *session.AdminID)
	}
	if session.AdminEmail != SystemActorEmail {
		t.Errorf("AdminEmail = %q, want %q", session.AdminEmail, SystemActorEmail)
	}

	if err := svc.EndImpersonation(session.ID); err != nil {
		t.Fatalf("end: %v", err)
	}
}

// TestStartImpersonation_UserAdmin_StillAttributed pins that the nil case did
// not weaken the normal path: a real admin is still recorded by id, and the
// tenant check still holds.
func TestStartImpersonation_UserAdmin_StillAttributed(t *testing.T) {
	svc, db, tn, target := seed(t)
	userService := user.NewService(db, zap.NewNop())

	admin, err := userService.Create(tn.ID, &user.CreateUserRequest{
		Email:    fmt.Sprintf("admin-%s@example.com", uuid.New().String()[:8]),
		Password: "correct-horse-battery",
		Name:     "Admin",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	defer db.Exec(`DELETE FROM users WHERE id = ?`, admin.ID)

	if _, err := svc.StartImpersonation(tn.ID, &admin.ID, &StartImpersonationRequest{
		TargetUserID: target.ID,
		Reason:       "a real admin must still be attributed",
	}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("start: %v", err)
	}

	var storedAdmin string
	if err := db.Raw(
		`SELECT admin_id::text FROM impersonation_sessions WHERE target_user_id = ?`, target.ID,
	).Scan(&storedAdmin).Error; err != nil {
		t.Fatalf("read session: %v", err)
	}
	if storedAdmin != admin.ID.String() {
		t.Errorf("admin_id = %q, want %q", storedAdmin, admin.ID)
	}
}
