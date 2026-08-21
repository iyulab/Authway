package tenant

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors the same-named helper already established in
// pkg/invitation and pkg/mfa — a real Postgres is required because this
// service relies on Postgres-specific behavior (JSONB Settings, soft-delete
// semantics via a real migrated schema) that an in-memory substitute cannot
// faithfully reproduce.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres tenant tests")
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

func newCreateReq(slug string) CreateTenantRequest {
	return CreateTenantRequest{
		Name: "Tenant " + slug,
		Slug: slug,
	}
}

func freshSlug(t *testing.T) string {
	t.Helper()
	return "tenant-test-" + uuid.New().String()[:8]
}

// TestCreateTenant_DuplicateActiveSlugRejected guards the basic uniqueness
// contract: a second CreateTenant against a slug that is still active must
// fail, not silently return (or overwrite) the existing row.
func TestCreateTenant_DuplicateActiveSlugRejected(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)
	slug := freshSlug(t)

	tn, err := svc.CreateTenant(newCreateReq(slug))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	if _, err := svc.CreateTenant(newCreateReq(slug)); !errors.Is(err, ErrDuplicateSlug) {
		t.Fatalf("expected ErrDuplicateSlug for a duplicate active slug, got %v", err)
	}
}

// TestCreateTenant_RestoresSoftDeletedTenant is the key regression guard for
// the restore-on-recreate branch in CreateTenant: recreating a slug whose
// only prior tenant was soft-deleted must resurrect that row (with the new
// request's data) rather than erroring, since soft-delete otherwise makes a
// slug permanently unusable.
func TestCreateTenant_RestoresSoftDeletedTenant(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)
	slug := freshSlug(t)

	original, err := svc.CreateTenant(newCreateReq(slug))
	if err != nil {
		t.Fatalf("CreateTenant (original): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, original.ID) })

	if err := svc.DeleteTenant(original.ID); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}
	if _, err := svc.GetTenantByID(original.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a soft-deleted tenant, got %v", err)
	}

	restored, err := svc.CreateTenant(CreateTenantRequest{Name: "Restored " + slug, Slug: slug})
	if err != nil {
		t.Fatalf("CreateTenant (restore): %v", err)
	}
	if restored.ID != original.ID {
		t.Fatalf("expected restore to reuse the original row ID %v, got %v", original.ID, restored.ID)
	}
	if restored.Name != "Restored "+slug {
		t.Fatalf("expected restore to apply the new request's Name, got %q", restored.Name)
	}
	if !restored.Active {
		t.Fatal("expected a restored tenant to be Active")
	}

	// The slug must be reachable again through the normal (scoped) lookup.
	found, err := svc.GetTenantBySlug(slug)
	if err != nil {
		t.Fatalf("GetTenantBySlug (post-restore): %v", err)
	}
	if found.ID != original.ID {
		t.Fatalf("expected GetTenantBySlug to resolve the restored tenant, got a different ID")
	}
}

// TestTenantLookups_ExcludeSoftDeleted guards that every scoped read path
// (GetTenantByID, GetTenantBySlug, ListTenants) treats a soft-deleted tenant
// as gone — a single forgotten `deleted_at IS NULL` clause would otherwise
// leak a deleted tenant back into an admin listing or lookup.
func TestTenantLookups_ExcludeSoftDeleted(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)
	slug := freshSlug(t)

	tn, err := svc.CreateTenant(newCreateReq(slug))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	if err := svc.DeleteTenant(tn.ID); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}

	if _, err := svc.GetTenantByID(tn.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTenantByID: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.GetTenantBySlug(slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTenantBySlug: expected ErrNotFound, got %v", err)
	}

	all, err := svc.ListTenants()
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	for _, listed := range all {
		if listed.ID == tn.ID {
			t.Fatalf("expected ListTenants to exclude the soft-deleted tenant %v", tn.ID)
		}
	}
}

// TestUpdateTenant_UpdatesFields is a straight-line sanity check on the
// non-default-tenant update path — every partial-update field actually lands.
func TestUpdateTenant_UpdatesFields(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)
	slug := freshSlug(t)

	tn, err := svc.CreateTenant(newCreateReq(slug))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	inactive := false
	updated, err := svc.UpdateTenant(tn.ID, UpdateTenantRequest{
		Name:        "Renamed " + slug,
		Description: "updated description",
		Active:      &inactive,
	})
	if err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}
	if updated.Name != "Renamed "+slug {
		t.Errorf("Name = %q, want %q", updated.Name, "Renamed "+slug)
	}
	if updated.Description != "updated description" {
		t.Errorf("Description = %q, want %q", updated.Description, "updated description")
	}
	if updated.Active {
		t.Error("expected Active to be false after explicit update")
	}
}

// TestDefaultTenant_CannotBeDeactivatedOrDeleted guards the two protections
// UpdateTenant/DeleteTenant carve out for the default tenant — losing either
// would let an operator accidentally lock every backward-compatible,
// unscoped-tenant caller out of the system.
//
// This resolves the live default tenant by slug, not by the DefaultTenantID
// constant: `000_initial_schema.sql` seeds the "default" tenant with a
// database-generated id (`gen_random_uuid()`), never DefaultTenantID, so on
// any normally-migrated database GetDefaultTenant()/DefaultTenantID do not
// resolve to the real row — only IsDefaultTenant()'s slug check does. See
// this cycle's log (Structural Improvement Proposals) for the inconsistency;
// this test intentionally exercises the path that is actually live.
func TestDefaultTenant_CannotBeDeactivatedOrDeleted(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)

	dt, err := svc.GetTenantBySlug("default")
	if err != nil {
		t.Fatalf("GetTenantBySlug(default): %v", err)
	}
	if !dt.IsDefaultTenant() {
		t.Fatalf("expected the migration-seeded 'default' tenant to satisfy IsDefaultTenant()")
	}

	inactive := false
	if _, err := svc.UpdateTenant(dt.ID, UpdateTenantRequest{Active: &inactive}); !errors.Is(err, ErrCannotDeactivateDefault) {
		t.Fatalf("expected ErrCannotDeactivateDefault, got %v", err)
	}
	if err := svc.DeleteTenant(dt.ID); !errors.Is(err, ErrCannotDeleteDefault) {
		t.Fatalf("expected ErrCannotDeleteDefault, got %v", err)
	}

	// Neither rejected call should have taken effect.
	after, err := svc.GetTenantBySlug("default")
	if err != nil {
		t.Fatalf("GetTenantBySlug(default) after rejected calls: %v", err)
	}
	if !after.Active {
		t.Fatal("expected the default tenant to remain Active after a rejected deactivation attempt")
	}
}

// TestEnsureDefaultTenant_Idempotent guards that repeated calls (every
// startup, per backfill.go-style convergence conventions elsewhere in this
// codebase) never produce a second "default"-slug tenant row.
//
// Counts by slug, not by DefaultTenantID — see the comment on
// TestDefaultTenant_CannotBeDeactivatedOrDeleted for why the constant does
// not identify the real row on a normally-migrated database.
func TestEnsureDefaultTenant_Idempotent(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)

	var before int64
	if err := db.Table("tenants").Where("slug = ?", "default").Count(&before).Error; err != nil {
		t.Fatalf("count default-slug rows (before): %v", err)
	}

	if err := svc.EnsureDefaultTenant(); err != nil {
		t.Fatalf("EnsureDefaultTenant (1st): %v", err)
	}
	if err := svc.EnsureDefaultTenant(); err != nil {
		t.Fatalf("EnsureDefaultTenant (2nd): %v", err)
	}

	var after int64
	if err := db.Table("tenants").Where("slug = ?", "default").Count(&after).Error; err != nil {
		t.Fatalf("count default-slug rows (after): %v", err)
	}
	if after != before {
		t.Fatalf("expected EnsureDefaultTenant to be a no-op when a 'default'-slug tenant already exists: before=%d after=%d", before, after)
	}
	if after != 1 {
		t.Fatalf("expected exactly one 'default'-slug tenant, got %d", after)
	}
}

// TestGetDefaultTenant_FallsBackToSlugWhenSeedIDDiffers reproduces the
// production bug: 000_initial_schema.sql seeds the default tenant with
// gen_random_uuid(), never DefaultTenantID, so a plain GetTenantByID(
// DefaultTenantID) lookup never finds it on any normally-migrated database.
// EnsureDefaultTenant and IsDefaultTenant() already fall back to the
// "default" slug (see service.go:194-213, models.go:132) — GetDefaultTenant
// was the one holdout.
func TestGetDefaultTenant_FallsBackToSlugWhenSeedIDDiffers(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)

	dt, err := svc.GetDefaultTenant()
	if err != nil {
		t.Fatalf("GetDefaultTenant(): %v", err)
	}
	if dt.Slug != "default" {
		t.Fatalf("expected the migration-seeded 'default' tenant, got slug=%q", dt.Slug)
	}
	if dt.ID == DefaultTenantID {
		t.Fatalf("test assumption violated: seeded default tenant unexpectedly has ID == DefaultTenantID; this test no longer exercises the fallback path")
	}
}

// TestDeleteTenant_BlockedByExistingUsersAndClients guards the two guard
// clauses DeleteTenant runs before it will soft-delete a tenant — without
// them, deleting a tenant orphans its users/clients (tenant_id pointing at a
// row that no longer resolves through any scoped lookup).
func TestDeleteTenant_BlockedByExistingUsersAndClients(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db)
	slug := freshSlug(t)

	tn, err := svc.CreateTenant(newCreateReq(slug))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	us := user.NewService(db, zap.NewNop())
	email := fmt.Sprintf("tenant-delete-guard-%s@example.com", uuid.New().String()[:8])
	u, err := us.Create(tn.ID, &user.CreateUserRequest{Email: email, Name: "Guard User", Password: "correct-horse-battery-staple"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := svc.DeleteTenant(tn.ID); !errors.Is(err, ErrHasUsers) {
		t.Fatalf("expected ErrHasUsers while an active user exists, got %v", err)
	}

	if err := db.Exec(`DELETE FROM users WHERE id = ?`, u.ID).Error; err != nil {
		t.Fatalf("cleanup user: %v", err)
	}

	clientID := uuid.New()
	if err := db.Exec(
		`INSERT INTO clients (id, tenant_id, client_id, client_secret, name) VALUES (?, ?, ?, ?, ?)`,
		clientID, tn.ID, "test-client-"+clientID.String()[:8], "secret", "Guard Client",
	).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}

	if err := svc.DeleteTenant(tn.ID); !errors.Is(err, ErrHasClients) {
		t.Fatalf("expected ErrHasClients while an active client exists, got %v", err)
	}

	if err := db.Exec(`DELETE FROM clients WHERE id = ?`, clientID).Error; err != nil {
		t.Fatalf("cleanup client: %v", err)
	}

	if err := svc.DeleteTenant(tn.ID); err != nil {
		t.Fatalf("DeleteTenant (after clearing users and clients): %v", err)
	}
}
