package user

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
)

// setupPostgres mirrors the same-named helper already established in
// pkg/invitation, pkg/mfa, and pkg/tenant — a real Postgres is required
// because User's tenant-scoped uniqueness (a composite index, not a plain
// unique column) is exactly the kind of constraint an in-memory substitute
// cannot faithfully enforce.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres user tests")
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

func freshTenant(t *testing.T, db *gorm.DB, ts *tenant.Service) *tenant.Tenant {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{
		Name: "user-test-" + suffix,
		Slug: "user-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
	return tn
}

func newTestService(t *testing.T, db *gorm.DB) (Service, *tenant.Service) {
	t.Helper()
	return NewService(db, zap.NewNop()), tenant.NewService(db)
}

// TestCreateUser_TenantScopedUniqueness guards the documented contract (see
// GetByEmailUnscoped's doc comment): the same email may exist in more than
// one tenant, but not twice within the same tenant.
func TestCreateUser_TenantScopedUniqueness(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tnA := freshTenant(t, db, ts)
	tnB := freshTenant(t, db, ts)
	email := fmt.Sprintf("dup-%s@example.com", uuid.New().String()[:8])

	u1, err := svc.Create(tnA.ID, &CreateUserRequest{Email: email, Name: "First"})
	if err != nil {
		t.Fatalf("Create (tenant A, 1st): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u1.ID) })

	if _, err := svc.Create(tnA.ID, &CreateUserRequest{Email: email, Name: "Second"}); err == nil {
		t.Fatal("expected Create to reject a duplicate email within the same tenant")
	}

	u2, err := svc.Create(tnB.ID, &CreateUserRequest{Email: email, Name: "Same email, other tenant"})
	if err != nil {
		t.Fatalf("expected the same email to be creatable in a different tenant, got: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u2.ID) })
}

// TestGetByEmailAndTenant_ScopesCorrectly is the direct regression guard for
// the login/verification-resend/password-reset tenant-scoping fix this
// codebase already shipped (CHANGELOG: "Login ... now all authenticate
// against the right tenant, not just the right email") — the same email in
// two tenants must resolve to the right row for each tenant, never the other.
func TestGetByEmailAndTenant_ScopesCorrectly(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tnA := freshTenant(t, db, ts)
	tnB := freshTenant(t, db, ts)
	email := fmt.Sprintf("scoped-%s@example.com", uuid.New().String()[:8])

	uA, err := svc.Create(tnA.ID, &CreateUserRequest{Email: email, Name: "Tenant A user"})
	if err != nil {
		t.Fatalf("Create (tenant A): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, uA.ID) })

	uB, err := svc.Create(tnB.ID, &CreateUserRequest{Email: email, Name: "Tenant B user"})
	if err != nil {
		t.Fatalf("Create (tenant B): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, uB.ID) })

	gotA, err := svc.GetByEmailAndTenant(tnA.ID, email)
	if err != nil {
		t.Fatalf("GetByEmailAndTenant (A): %v", err)
	}
	if gotA.ID != uA.ID {
		t.Fatalf("GetByEmailAndTenant(tenant A) resolved the wrong user: got %v, want %v", gotA.ID, uA.ID)
	}

	gotB, err := svc.GetByEmailAndTenant(tnB.ID, email)
	if err != nil {
		t.Fatalf("GetByEmailAndTenant (B): %v", err)
	}
	if gotB.ID != uB.ID {
		t.Fatalf("GetByEmailAndTenant(tenant B) resolved the wrong user: got %v, want %v", gotB.ID, uB.ID)
	}
}

// TestPasswordLifecycle_VerifyChangeReset covers the three password paths
// together: VerifyPassword against the hash Create produced, ChangePassword's
// current-password gate, and UpdatePassword's no-gate reset path — each
// feeding the next so a break in the hashing contract anywhere in the chain
// surfaces as a failure here rather than three independently-passing tests
// that each construct their own (possibly inconsistent) hash.
func TestPasswordLifecycle_VerifyChangeReset(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	email := fmt.Sprintf("pw-%s@example.com", uuid.New().String()[:8])

	u, err := svc.Create(tn.ID, &CreateUserRequest{Email: email, Name: "Password Test", Password: "initial-password-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	if !svc.VerifyPassword(u, "initial-password-1") {
		t.Fatal("expected VerifyPassword to accept the password Create hashed")
	}
	if svc.VerifyPassword(u, "wrong-password") {
		t.Fatal("expected VerifyPassword to reject an incorrect password")
	}

	if err := svc.ChangePassword(u.ID, &ChangePasswordRequest{CurrentPassword: "wrong-password", NewPassword: "new-password-2"}); err == nil {
		t.Fatal("expected ChangePassword to reject an incorrect current password")
	}
	if err := svc.ChangePassword(u.ID, &ChangePasswordRequest{CurrentPassword: "initial-password-1", NewPassword: "new-password-2"}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	reloaded, err := svc.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID (post-ChangePassword): %v", err)
	}
	if !svc.VerifyPassword(reloaded, "new-password-2") {
		t.Fatal("expected VerifyPassword to accept the password set by ChangePassword")
	}
	if svc.VerifyPassword(reloaded, "initial-password-1") {
		t.Fatal("expected the old password to stop working after ChangePassword")
	}

	if err := svc.UpdatePassword(u.ID, "reset-password-3"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	reloaded, err = svc.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID (post-UpdatePassword): %v", err)
	}
	if !svc.VerifyPassword(reloaded, "reset-password-3") {
		t.Fatal("expected VerifyPassword to accept the password set by UpdatePassword")
	}
}

// TestVerifyPassword_SocialLoginUserHasNoPassword guards that a social-login
// account (created with no password, PasswordHash == "") never validates any
// password — bcrypt.CompareHashAndPassword against an empty hash is exactly
// the kind of edge case that can panic or (worse) accidentally pass.
func TestVerifyPassword_SocialLoginUserHasNoPassword(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	email := fmt.Sprintf("social-%s@example.com", uuid.New().String()[:8])

	u, err := svc.Create(tn.ID, &CreateUserRequest{Email: email, Name: "Social User"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	if u.PasswordHash != "" {
		t.Fatalf("expected a passwordless Create to leave PasswordHash empty, got %q", u.PasswordHash)
	}
	if svc.VerifyPassword(u, "") {
		t.Fatal("expected VerifyPassword to reject an empty password against a social-login account")
	}
	if svc.VerifyPassword(u, "anything") {
		t.Fatal("expected VerifyPassword to reject any password against a social-login account")
	}
}

// TestUpdate_EmptyFieldsLeaveExistingValuesUnchanged guards Update's explicit
// "empty string means not provided" contract (unlike tenant.UpdateTenant's
// pointer fields) — an empty AvatarURL in the request must not clear a
// previously-set one.
func TestUpdate_EmptyFieldsLeaveExistingValuesUnchanged(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	email := fmt.Sprintf("update-%s@example.com", uuid.New().String()[:8])

	u, err := svc.Create(tn.ID, &CreateUserRequest{Email: email, Name: "Original Name"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	if _, err := svc.Update(u.ID, &UpdateUserRequest{Name: "Updated Name", AvatarURL: "https://example.com/a.png"}); err != nil {
		t.Fatalf("Update (set avatar): %v", err)
	}

	updated, err := svc.Update(u.ID, &UpdateUserRequest{})
	if err != nil {
		t.Fatalf("Update (empty request): %v", err)
	}
	if updated.Name == nil || *updated.Name != "Updated Name" {
		t.Fatalf("expected Name to remain %q after an empty-field update, got %v", "Updated Name", updated.Name)
	}
	if updated.AvatarURL == nil || *updated.AvatarURL != "https://example.com/a.png" {
		t.Fatalf("expected AvatarURL to remain set after an empty-field update, got %v", updated.AvatarURL)
	}
}

// TestDelete_SoftDeletesAndSecondCallReportsNotFound guards Delete's
// RowsAffected-based not-found signal — without it, a second Delete on an
// already-deleted (or never-existing) ID would silently report success.
func TestDelete_SoftDeletesAndSecondCallReportsNotFound(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	email := fmt.Sprintf("delete-%s@example.com", uuid.New().String()[:8])

	u, err := svc.Create(tn.ID, &CreateUserRequest{Email: email, Name: "Delete Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	if err := svc.Delete(u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.GetByID(u.ID); err == nil {
		t.Fatal("expected GetByID to fail for a soft-deleted user")
	}
	if err := svc.Delete(u.ID); err == nil {
		t.Fatal("expected a second Delete on an already-deleted user to report not-found, not succeed silently")
	}
}

// TestGetByTenant_ScopesAndPaginates guards that listing is both correctly
// tenant-scoped (a user in tenant B must never appear in tenant A's list)
// and correctly paginated (total reflects the full scoped count, not the
// page size).
func TestGetByTenant_ScopesAndPaginates(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tnA := freshTenant(t, db, ts)
	tnB := freshTenant(t, db, ts)

	for i := 0; i < 3; i++ {
		email := fmt.Sprintf("tenanta-%d-%s@example.com", i, uuid.New().String()[:8])
		u, err := svc.Create(tnA.ID, &CreateUserRequest{Email: email, Name: "A"})
		if err != nil {
			t.Fatalf("Create (tenant A, %d): %v", i, err)
		}
		t.Cleanup(func(id uuid.UUID) func() { return func() { db.Exec(`DELETE FROM users WHERE id = ?`, id) } }(u.ID))
	}
	uB, err := svc.Create(tnB.ID, &CreateUserRequest{Email: fmt.Sprintf("tenantb-%s@example.com", uuid.New().String()[:8]), Name: "B"})
	if err != nil {
		t.Fatalf("Create (tenant B): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, uB.ID) })

	page, total, err := svc.GetByTenant(tnA.ID, 2, 0)
	if err != nil {
		t.Fatalf("GetByTenant (tenant A, page 1): %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3 for tenant A, got %d", total)
	}
	if len(page) != 2 {
		t.Fatalf("expected a 2-item page, got %d", len(page))
	}
	for _, u := range page {
		if u.TenantID != tnA.ID {
			t.Fatalf("GetByTenant(tenant A) returned a user from another tenant: %v", u.TenantID)
		}
	}

	restB, totalB, err := svc.GetByTenant(tnB.ID, 10, 0)
	if err != nil {
		t.Fatalf("GetByTenant (tenant B): %v", err)
	}
	if totalB != 1 || len(restB) != 1 {
		t.Fatalf("expected exactly 1 user in tenant B, got total=%d len=%d", totalB, len(restB))
	}
}
