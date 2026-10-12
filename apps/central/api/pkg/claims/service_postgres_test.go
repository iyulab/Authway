package claims

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors the same-named helper already established in
// pkg/invitation, pkg/mfa, pkg/tenant, and pkg/user — the permanent-claim
// half of this service is Postgres-backed (JSONB storage, a composite unique
// index on user_id/tenant_id/claim_key).
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

// newTestRedis mirrors pkg/middleware's ratelimit_test.go helper of the same
// intent — an in-process miniredis instance, so the pending/login-claims
// half of this service (Redis-backed) is exercised against something
// Redis-shaped without a real Redis dependency.
func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func newTestService(t *testing.T, db *gorm.DB) Service {
	t.Helper()
	repo := NewRepository(db, newTestRedis(t))
	return NewService(repo, zap.NewNop())
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
	repo := NewRepository(db, newTestRedis(t))
	svc := NewService(repo, zap.NewNop())
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)
	if err := repo.SaveClaim(ctx, &UserClaim{
		ID: uuid.New(), UserID: userID, TenantID: tenantID, ClaimKey: "tenant_id",
		ClaimValue: map[string]any{"value": "other"}, IsPermanent: true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := svc.Patch(ctx, userID, tenantID, map[string]any{"tenant_id": nil}); err != nil {
		t.Fatalf("Patch (remove reserved): %v", err)
	}
	var rows int64
	db.Model(&UserClaim{}).Where("user_id = ? AND claim_key = ?", userID, "tenant_id").Count(&rows)
	if rows != 0 {
		t.Fatalf("the stored reserved claim is still there (%d rows)", rows)
	}
}

// TestGetClaimsForConsent_PrefersLoginChallengeThenDatabase exercises the
// fallback GetClaimsForConsent documents: the claims prepared for the login
// challenge first, the stored claims when there are none.
func TestGetClaimsForConsent_PrefersLoginChallengeThenDatabase(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)
	if _, err := svc.Patch(ctx, userID, tenantID, map[string]any{"plan": "pro"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	claims, source, err := svc.GetClaimsForConsent(ctx, "challenge-with-no-data", userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent (database fallback): %v", err)
	}
	if source != ClaimsSourceDatabase || claims["plan"] != "pro" {
		t.Fatalf("database fallback: source=%v claims=%+v", source, claims)
	}

	loginChallenge := "challenge-" + uuid.New().String()[:8]
	loginClaims, err := svc.GetClaimsForLogin(ctx, userID, tenantID, loginChallenge)
	if err != nil {
		t.Fatalf("GetClaimsForLogin: %v", err)
	}
	if loginClaims["plan"] != "pro" {
		t.Fatalf("expected GetClaimsForLogin to carry the stored claim forward, got %+v", loginClaims)
	}
	viaChallenge, source, err := svc.GetClaimsForConsent(ctx, loginChallenge, userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent (login challenge): %v", err)
	}
	if source != ClaimsSourceLoginChallenge || viaChallenge["plan"] != "pro" {
		t.Fatalf("login challenge: source=%v claims=%+v", source, viaChallenge)
	}
}

// TestGetClaimsForConsent_NoClaimsAnywhereReportsSourceNone guards the final
// fallback rung: with no login-challenge data, no pending claims and no
// stored claims, the result must still be a valid (empty, source=none) map
// rather than an error or a nil claims payload a caller would need to
// nil-check.
func TestGetClaimsForConsent_NoClaimsAnywhereReportsSourceNone(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	claims, source, err := svc.GetClaimsForConsent(ctx, "nonexistent-challenge", userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent: %v", err)
	}
	if source != ClaimsSourceNone || claims == nil || len(claims) != 0 {
		t.Fatalf("expected an empty map with source=none, got %+v (%v)", claims, source)
	}
}

// TestStoredReservedClaimsNeverReachATokenPath: a reserved claim stored
// before writes were checked (or put there by any other means) is left out of
// every path that feeds a token, and out of what the claims API shows.
func TestStoredReservedClaimsNeverReachATokenPath(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	repo := NewRepository(db, newTestRedis(t))
	svc := NewService(repo, zap.NewNop())
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	for key, value := range map[string]any{"tenant_id": uuid.New().String(), "email_verified": true, "_source": "pending", "plan": "pro"} {
		if err := repo.SaveClaim(ctx, &UserClaim{
			ID: uuid.New(), UserID: userID, TenantID: tenantID, ClaimKey: key,
			ClaimValue: map[string]any{"value": value}, ClaimType: "user", IsPermanent: true,
		}); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	if err := repo.SetPendingClaims(ctx, userID, ClaimMap{"email": "attacker@example.com"}, time.Minute); err != nil {
		t.Fatalf("seed pending: %v", err)
	}

	onlyPlan := func(where string, claims ClaimMap) {
		t.Helper()
		if len(claims) != 1 || claims["plan"] != "pro" {
			t.Fatalf("%s: want only plan=pro, got %+v", where, claims)
		}
	}

	viaPending, source, err := svc.GetClaimsForConsent(ctx, "no-such-challenge", userID, tenantID)
	if err != nil || source != ClaimsSourcePending {
		t.Fatalf("GetClaimsForConsent (pending): source=%v err=%v", source, err)
	}
	onlyPlan("consent via pending", viaPending)

	viaDatabase, _, err := svc.GetClaimsForConsent(ctx, "no-such-challenge", userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent (database): %v", err)
	}
	onlyPlan("consent via database", viaDatabase)

	challenge := "challenge-" + uuid.New().String()[:8]
	forLogin, err := svc.GetClaimsForLogin(ctx, userID, tenantID, challenge)
	if err != nil {
		t.Fatalf("GetClaimsForLogin: %v", err)
	}
	onlyPlan("login", forLogin)
	viaChallenge, _, err := svc.GetClaimsForConsent(ctx, challenge, userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent (challenge): %v", err)
	}
	onlyPlan("consent via login challenge", viaChallenge)

	shown, err := svc.List(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	onlyPlan("GET /users/{id}/claims", shown)
}
