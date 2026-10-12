package claims

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors the same-named helper already established in
// pkg/invitation, pkg/mfa, pkg/tenant, and pkg/user — claims are stored in
// Postgres (JSONB values, a composite unique index on
// user_id/tenant_id/claim_key).
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres claims tests")
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

func newTestService(t *testing.T, db *gorm.DB) Service {
	t.Helper()
	return NewService(NewRepository(db), zap.NewNop())
}

// seedClaim stores a claim as a row, past the service's checks — the way a
// claim written before those checks existed sits in the table.
func seedClaim(t *testing.T, db *gorm.DB, userID, tenantID uuid.UUID, key string, value any) {
	t.Helper()
	if err := db.Create(&UserClaim{
		ID: uuid.New(), UserID: userID, TenantID: tenantID, ClaimKey: key,
		ClaimValue: map[string]any{"value": value},
	}).Error; err != nil {
		t.Fatalf("seed %s: %v", key, err)
	}
}

// fixtureUser creates a real tenant + user row — user_claims.user_id carries
// a foreign key to users(id), so a fabricated uuid.New() with no backing row
// fails every insert with a constraint violation rather than exercising the
// claims logic this test actually targets.
func fixtureUser(t *testing.T, db *gorm.DB) (userID, tenantID uuid.UUID) {
	t.Helper()
	ts := tenant.NewService(db)
	suffix := uuid.New().String()[:8]
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{Name: "claims-test-" + suffix, Slug: "claims-test-" + suffix})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	us := user.NewService(db, zap.NewNop())
	u, err := us.Create(tn.ID, &user.CreateUserRequest{
		Email: fmt.Sprintf("claims-%s@example.com", suffix), Name: "Claims Test User",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	return u.ID, tn.ID
}

func cleanupUserClaims(t *testing.T, db *gorm.DB, userID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() { db.Exec(`DELETE FROM user_claims WHERE user_id = ?`, userID) })
}

// TestPatch_SetsAndRemovesClaims: a merge patch sets the claims it names
// with a value, removes those it names with null, and leaves the rest.
func TestPatch_SetsAndRemovesClaims(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	got, err := svc.Patch(ctx, userID, tenantID, map[string]any{"plan": "pro", "org": "acme"})
	if err != nil {
		t.Fatalf("Patch (set): %v", err)
	}
	if len(got) != 2 || got["plan"] != "pro" || got["org"] != "acme" {
		t.Fatalf("Patch (set) returned %+v", got)
	}

	got, err = svc.Patch(ctx, userID, tenantID, map[string]any{"org": nil, "seats": float64(5), "plan": "team"})
	if err != nil {
		t.Fatalf("Patch (mixed): %v", err)
	}
	stored, err := svc.List(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for name, claims := range map[string]ClaimMap{"returned": got, "stored": stored} {
		if len(claims) != 2 || claims["plan"] != "team" || claims["seats"] != float64(5) {
			t.Fatalf("%s after the second patch: %+v", name, claims)
		}
	}

	// Removing a claim that is not stored is not an error.
	if _, err := svc.Patch(ctx, userID, tenantID, map[string]any{"absent": nil}); err != nil {
		t.Fatalf("Patch (remove absent): %v", err)
	}
}

// TestPatch_RefusalsChangeNothing: a patch naming a reserved claim, an
// unusable name, or making the claims too large for a token is refused as a
// whole.
func TestPatch_RefusalsChangeNothing(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)
	if _, err := svc.Patch(ctx, userID, tenantID, map[string]any{"plan": "pro"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := svc.Patch(ctx, userID, tenantID, map[string]any{"tenant_id": uuid.New().String(), "sub": "x", "plan": nil})
	var reservedErr *ReservedClaimError
	if !errors.As(err, &reservedErr) || strings.Join(reservedErr.Keys, ",") != "sub,tenant_id" {
		t.Fatalf("reserved: want ReservedClaimError{sub,tenant_id}, got %v", err)
	}

	_, err = svc.Patch(ctx, userID, tenantID, map[string]any{"": "x", "plan": nil})
	var invalidErr *InvalidClaimError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("empty name: want InvalidClaimError, got %v", err)
	}

	_, err = svc.Patch(ctx, userID, tenantID, map[string]any{"notes": strings.Repeat("x", maxClaimsSize), "plan": nil})
	var tooLargeErr *ClaimsTooLargeError
	if !errors.As(err, &tooLargeErr) {
		t.Fatalf("too large: want ClaimsTooLargeError, got %v", err)
	}

	stored, err := svc.List(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stored) != 1 || stored["plan"] != "pro" {
		t.Fatalf("a refused patch changed the stored claims: %+v", stored)
	}
}

// TestPatch_RemovesAStoredReservedClaim: a reserved claim stored before
// writes were checked can be removed, though never set.
func TestPatch_RemovesAStoredReservedClaim(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)
	seedClaim(t, db, userID, tenantID, "tenant_id", "other")

	if _, err := svc.Patch(ctx, userID, tenantID, map[string]any{"tenant_id": nil}); err != nil {
		t.Fatalf("Patch (remove reserved): %v", err)
	}
	var rows int64
	db.Model(&UserClaim{}).Where("user_id = ? AND claim_key = ?", userID, "tenant_id").Count(&rows)
	if rows != 0 {
		t.Fatalf("the stored reserved claim is still there (%d rows)", rows)
	}
}

// TestList_LeavesOutStoredReservedClaims: List is what the tokens are built
// from and what the claims API shows; a reserved claim stored before writes
// were checked never comes out of it.
func TestList_LeavesOutStoredReservedClaims(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)
	for key, value := range map[string]any{"tenant_id": uuid.New().String(), "email_verified": true, "_source": "pending", "plan": "pro"} {
		seedClaim(t, db, userID, tenantID, key, value)
	}

	stored, err := svc.List(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stored) != 1 || stored["plan"] != "pro" {
		t.Fatalf("want only plan=pro, got %+v", stored)
	}
}

// TestList_NoClaimsIsAnEmptyMap: a user with nothing stored gets an empty
// map, not nil — callers merge into it.
func TestList_NoClaimsIsAnEmptyMap(t *testing.T) {
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)

	stored, err := svc.List(context.Background(), userID, tenantID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if stored == nil || len(stored) != 0 {
		t.Fatalf("want an empty map, got %#v", stored)
	}
}

// TestWritesDoNotNameTheRetiredColumns: claim_type and is_permanent are
// mapped read-only until a migration drops them. A write that named them
// would fail on the instances still running while that migration is applied.
func TestWritesDoNotNameTheRetiredColumns(t *testing.T) {
	db := setupPostgres(t)
	stmt := db.Session(&gorm.Session{DryRun: true}).Create(&UserClaim{
		ID: uuid.New(), UserID: uuid.New(), TenantID: uuid.New(), ClaimKey: "plan",
		ClaimValue: map[string]any{"value": "pro"},
	}).Statement
	sql := stmt.SQL.String()
	if !strings.Contains(sql, "claim_key") {
		t.Fatalf("not the insert this test means to read: %s", sql)
	}
	for _, column := range []string{"claim_type", "is_permanent"} {
		if strings.Contains(sql, column) {
			t.Fatalf("the insert names %s: %s", column, sql)
		}
	}
}
