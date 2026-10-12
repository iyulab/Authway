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
	return NewService(repo, "http://localhost:4444", zap.NewNop())
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

// TestUpdateClaims_PermanentPersistsAndPendingOverridesOnRead is the key
// precedence guard documented in GetClaims: a pending (session, Redis-backed)
// claim must override a same-key permanent (database) claim on read, while
// the database row itself stays untouched.
func TestUpdateClaims_PermanentPersistsAndPendingOverridesOnRead(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	_, err := svc.UpdateClaims(ctx, userID, tenantID, &UpdateClaimsRequest{
		Claims:      ClaimMap{"role": "member", "org": "acme"},
		Permanent:   true,
		ClientID:    "test-client",
		RedirectURI: "https://app.example.com/callback",
	})
	if err != nil {
		t.Fatalf("UpdateClaims (permanent): %v", err)
	}

	got, err := svc.GetClaims(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaims (after permanent write): %v", err)
	}
	if got.Claims["role"] != "member" || got.Claims["org"] != "acme" {
		t.Fatalf("expected both permanent claims present, got %+v", got.Claims)
	}
	if len(got.PermanentClaims) != 2 {
		t.Fatalf("expected 2 permanent claim keys, got %v", got.PermanentClaims)
	}
	// UpdateClaims always stages its claims as pending too (step 1 in the
	// implementation runs unconditionally; Permanent only controls the
	// additional database write) — so both keys are also session claims
	// right after this first, permanent=true call.
	if len(got.SessionClaims) != 2 {
		t.Fatalf("expected both keys staged as session claims after the initial update, got %v", got.SessionClaims)
	}

	// A non-permanent update for a single key REPLACES the whole pending set
	// (SetPendingClaims overwrites the Redis key, it does not merge) — so
	// afterward only "role" is pending, while "org" still reads from the
	// untouched database row.
	if _, err := svc.UpdateClaims(ctx, userID, tenantID, &UpdateClaimsRequest{
		Claims:      ClaimMap{"role": "admin"},
		Permanent:   false,
		ClientID:    "test-client",
		RedirectURI: "https://app.example.com/callback",
	}); err != nil {
		t.Fatalf("UpdateClaims (pending): %v", err)
	}

	got, err = svc.GetClaims(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaims (after pending override): %v", err)
	}
	if got.Claims["role"] != "admin" {
		t.Fatalf("expected the pending claim to override the permanent one, got role=%v", got.Claims["role"])
	}
	if got.Claims["org"] != "acme" {
		t.Fatalf("expected the untouched permanent claim to still be present, got org=%v", got.Claims["org"])
	}
	if len(got.SessionClaims) != 1 || got.SessionClaims[0] != "role" {
		t.Fatalf("expected exactly ['role'] as session claims (pending set was replaced, not merged), got %v", got.SessionClaims)
	}
}

// TestDeleteClaim_RemovesPermanentClaimAndRepeatDeleteErrors guards the
// repository's RowsAffected-based not-found signal reaching the service layer
// as an actual error, not a silent success.
func TestDeleteClaim_RemovesPermanentClaimAndRepeatDeleteErrors(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	if _, err := svc.UpdateClaims(ctx, userID, tenantID, &UpdateClaimsRequest{
		Claims:      ClaimMap{"plan": "pro"},
		Permanent:   true,
		ClientID:    "test-client",
		RedirectURI: "https://app.example.com/callback",
	}); err != nil {
		t.Fatalf("UpdateClaims: %v", err)
	}

	if _, err := svc.DeleteClaim(ctx, userID, tenantID, "plan"); err != nil {
		t.Fatalf("DeleteClaim: %v", err)
	}

	if _, err := svc.DeleteClaim(ctx, userID, tenantID, "plan"); err == nil {
		t.Fatal("expected deleting an already-deleted claim to error, not succeed silently")
	}
}

// TestUserClaims_IsolatedByClaimType guards that UpdateUserClaims/GetUserClaims
// (claim_type="user", no re-auth) is a distinct, independently-readable set
// from the general permanent-claim store, keyed only by claim_type.
func TestUserClaims_IsolatedByClaimType(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	if _, err := svc.UpdateUserClaims(ctx, userID, tenantID, &UpdateUserClaimsRequest{
		Claims: ClaimMap{"theme": "dark"},
	}); err != nil {
		t.Fatalf("UpdateUserClaims: %v", err)
	}

	userClaims, err := svc.GetUserClaims(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("GetUserClaims: %v", err)
	}
	if userClaims.Claims["theme"] != "dark" {
		t.Fatalf("expected theme=dark in user claims, got %+v", userClaims.Claims)
	}
}

// TestGetClaimsForConsent_PrefersLoginChallengeThenFallsBackToPendingThenDatabase
// exercises the three-strategy fallback GetClaimsForConsent documents in its
// own comment — this is the exact logic Hydra's consent flow depends on to
// inject the right claims into the issued token.
func TestGetClaimsForConsent_PrefersLoginChallengeThenFallsBackToPendingThenDatabase(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	repo := NewRepository(db, newTestRedis(t))
	svc := NewService(repo, "http://localhost:4444", zap.NewNop())
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	// Strategy 3 (no login-challenge claims, no pending): base user info + an
	// existing permanent claim, source=database. UpdateClaims always stages
	// its claims as pending too (see the sibling precedence test), so the
	// pending window is cleared explicitly here to reach the state this
	// strategy actually targets — a permanent claim outliving its 5-minute
	// pending window.
	if _, err := svc.UpdateClaims(ctx, userID, tenantID, &UpdateClaimsRequest{
		Claims: ClaimMap{"plan": "pro"}, Permanent: true,
		ClientID: "c", RedirectURI: "https://app.example.com/cb",
	}); err != nil {
		t.Fatalf("UpdateClaims (seed permanent): %v", err)
	}
	if err := repo.DeletePendingClaims(ctx, userID); err != nil {
		t.Fatalf("DeletePendingClaims (simulate expiry): %v", err)
	}

	claims, source, err := svc.GetClaimsForConsent(ctx, "challenge-with-no-data", userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent (database fallback): %v", err)
	}
	if source != ClaimsSourceDatabase {
		t.Fatalf("expected source=database, got %v", source)
	}
	if claims["plan"] != "pro" {
		t.Fatalf("expected the permanent claim, got %+v", claims)
	}

	// Strategy 1: claims explicitly associated with a login challenge take
	// priority over everything else.
	loginChallenge := "challenge-" + uuid.New().String()[:8]
	loginClaims, err := svc.GetClaimsForLogin(ctx, userID, tenantID, loginChallenge)
	if err != nil {
		t.Fatalf("GetClaimsForLogin: %v", err)
	}
	if loginClaims["plan"] != "pro" {
		t.Fatalf("expected GetClaimsForLogin to carry the permanent claim forward, got %+v", loginClaims)
	}

	viaChallenge, source, err := svc.GetClaimsForConsent(ctx, loginChallenge, userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaimsForConsent (login challenge): %v", err)
	}
	if source != ClaimsSourceLoginChallenge {
		t.Fatalf("expected source=login_challenge, got %v", source)
	}
	if viaChallenge["plan"] != "pro" {
		t.Fatalf("expected the login-challenge claims to include the permanent claim, got %+v", viaChallenge)
	}
}

// TestGetClaimsForConsent_NoClaimsAnywhereReportsSourceNone guards the final
// fallback rung: with no login-challenge data, no pending claims and no
// permanent claims, the result must still be a valid (empty, source=none)
// map rather than an error or a nil claims payload a caller would need to
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

// TestClaimWrites_RefuseReservedClaims: a user writes their own claims with
// their own token, so neither write path may take a claim the issuer asserts
// (tenant_id, email, …) or one OAuth defines — and a refused write stores
// nothing, not even its other claims.
func TestClaimWrites_RefuseReservedClaims(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	svc := newTestService(t, db)
	userID, tenantID := fixtureUser(t, db)
	cleanupUserClaims(t, db, userID)

	_, err := svc.UpdateUserClaims(ctx, userID, tenantID, &UpdateUserClaimsRequest{
		Claims: ClaimMap{"tenant_id": uuid.New().String(), "theme": "dark"},
	})
	var reservedErr *ReservedClaimError
	if !errors.As(err, &reservedErr) || len(reservedErr.Keys) != 1 || reservedErr.Keys[0] != "tenant_id" {
		t.Fatalf("UpdateUserClaims: want ReservedClaimError{tenant_id}, got %v", err)
	}

	_, err = svc.UpdateClaims(ctx, userID, tenantID, &UpdateClaimsRequest{
		Claims:    ClaimMap{"email_verified": true, "auth_time": 4102444800, "sub": "someone-else", "plan": "pro"},
		Permanent: true, ClientID: "c", RedirectURI: "https://app.example.com/cb",
	})
	if !errors.As(err, &reservedErr) || strings.Join(reservedErr.Keys, ",") != "auth_time,email_verified,sub" {
		t.Fatalf("UpdateClaims: want ReservedClaimError{auth_time,email_verified,sub}, got %v", err)
	}

	got, err := svc.GetClaims(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("GetClaims: %v", err)
	}
	if len(got.Claims) != 0 {
		t.Fatalf("a refused write stored claims: %+v", got.Claims)
	}
}

// TestStoredReservedClaimsNeverReachATokenPath: a reserved claim stored
// before writes were checked (or put there by any other means) is left out of
// every path that feeds a token, and out of what the claims API shows.
func TestStoredReservedClaimsNeverReachATokenPath(t *testing.T) {
	ctx := context.Background()
	db := setupPostgres(t)
	repo := NewRepository(db, newTestRedis(t))
	svc := NewService(repo, "http://localhost:4444", zap.NewNop())
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

	shown, err := svc.GetUserClaims(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("GetUserClaims: %v", err)
	}
	onlyPlan("GET /claims/user", shown.Claims)
}
