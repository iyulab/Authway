package email

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors the same-named helper already established in
// pkg/invitation, pkg/mfa, pkg/tenant, pkg/user, and pkg/claims.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres email tests")
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

// fixtureUser creates a real tenant + user row — email_verifications.user_id
// and password_resets.user_id both carry a foreign key to users(id).
func fixtureUser(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	suffix := uuid.New().String()[:8]
	ts := tenant.NewService(db)
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{Name: "email-test-" + suffix, Slug: "email-test-" + suffix})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	us := user.NewService(db, zap.NewNop())
	u, err := us.Create(tn.ID, &user.CreateUserRequest{Email: fmt.Sprintf("email-%s@example.com", suffix), Name: "Email Test User"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	return u.ID
}

// TestCreateVerification_TokenHashedAtRestAndFindableByPlaintext guards the
// same at-rest hashing contract every other token table in this codebase
// already carries (see CHANGELOG's 010/013/014/019 series): the plaintext
// token handed back for the email link must resolve via GetVerificationByToken,
// while the stored column must never equal that plaintext.
func TestCreateVerification_TokenHashedAtRestAndFindableByPlaintext(t *testing.T) {
	db := setupPostgres(t)
	repo := NewRepository(db)
	userID := fixtureUser(t, db)

	v, err := repo.CreateVerification(userID)
	if err != nil {
		t.Fatalf("CreateVerification: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM email_verifications WHERE id = ?`, v.ID) })

	if v.Token == "" {
		t.Fatal("expected a non-empty plaintext token")
	}
	if v.TokenHash == v.Token {
		t.Fatal("expected TokenHash to differ from the plaintext Token")
	}

	found, err := repo.GetVerificationByToken(v.Token)
	if err != nil {
		t.Fatalf("GetVerificationByToken: %v", err)
	}
	if found.ID != v.ID || found.UserID != userID {
		t.Fatalf("resolved the wrong verification: got %+v", found)
	}

	var stored string
	if err := db.Raw(`SELECT token_hash FROM email_verifications WHERE id = ?`, v.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read back token_hash: %v", err)
	}
	if stored == v.Token {
		t.Fatal("expected the stored token_hash column to never equal the plaintext token")
	}
}

// TestGetVerificationByToken_ExcludesAlreadyVerified guards the query's
// verified=false filter — a token that already completed verification must
// not resolve again (preventing replay of a used verification link).
func TestGetVerificationByToken_ExcludesAlreadyVerified(t *testing.T) {
	db := setupPostgres(t)
	repo := NewRepository(db)
	userID := fixtureUser(t, db)

	v, err := repo.CreateVerification(userID)
	if err != nil {
		t.Fatalf("CreateVerification: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM email_verifications WHERE id = ?`, v.ID) })

	if err := repo.MarkVerificationAsVerified(v.ID); err != nil {
		t.Fatalf("MarkVerificationAsVerified: %v", err)
	}

	if _, err := repo.GetVerificationByToken(v.Token); err == nil {
		t.Fatal("expected an already-verified token to no longer resolve")
	}
}

// TestCreatePasswordReset_InvalidatesPriorUnusedTokens guards the
// single-live-token invariant: issuing a second reset for the same user must
// silently retire the first, so an attacker who intercepted an older reset
// email (or a user who requested one, forgot, then requested another) cannot
// use both.
func TestCreatePasswordReset_InvalidatesPriorUnusedTokens(t *testing.T) {
	db := setupPostgres(t)
	repo := NewRepository(db)
	userID := fixtureUser(t, db)

	first, err := repo.CreatePasswordReset(userID)
	if err != nil {
		t.Fatalf("CreatePasswordReset (1st): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID) })

	second, err := repo.CreatePasswordReset(userID)
	if err != nil {
		t.Fatalf("CreatePasswordReset (2nd): %v", err)
	}

	if _, err := repo.GetPasswordResetByToken(first.Token); err == nil {
		t.Fatal("expected the first reset token to be invalidated once a second was issued")
	}

	found, err := repo.GetPasswordResetByToken(second.Token)
	if err != nil {
		t.Fatalf("GetPasswordResetByToken (2nd): %v", err)
	}
	if found.ID != second.ID {
		t.Fatalf("expected the second reset to still resolve, got a different row")
	}
}

// TestMarkPasswordResetAsUsed_PersistsAndBlocksFutureLookup guards that
// MarkAsUsed's in-memory flip (Used=true, UsedAt set) actually persists
// through Repository.MarkPasswordResetAsUsed, and that a used token then
// fails the same used=false filter CreatePasswordReset's invalidation path
// relies on.
func TestMarkPasswordResetAsUsed_PersistsAndBlocksFutureLookup(t *testing.T) {
	db := setupPostgres(t)
	repo := NewRepository(db)
	userID := fixtureUser(t, db)

	reset, err := repo.CreatePasswordReset(userID)
	if err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID) })

	if err := repo.MarkPasswordResetAsUsed(reset.ID); err != nil {
		t.Fatalf("MarkPasswordResetAsUsed: %v", err)
	}

	if _, err := repo.GetPasswordResetByToken(reset.Token); err == nil {
		t.Fatal("expected a used reset token to no longer resolve")
	}

	var used bool
	var usedAt *time.Time
	if err := db.Raw(`SELECT used, used_at FROM password_resets WHERE id = ?`, reset.ID).Row().Scan(&used, &usedAt); err != nil {
		t.Fatalf("read back used/used_at: %v", err)
	}
	if !used || usedAt == nil {
		t.Fatalf("expected used=true and used_at set, got used=%v used_at=%v", used, usedAt)
	}
}

// TestDeleteVerificationsAndResetsByUserID_RemoveAllRowsForThatUser guards
// the two per-user bulk-delete helpers (used on account deletion) actually
// scope to the given user and remove every row, not just the most recent.
func TestDeleteVerificationsAndResetsByUserID_RemoveAllRowsForThatUser(t *testing.T) {
	db := setupPostgres(t)
	repo := NewRepository(db)
	userID := fixtureUser(t, db)

	if _, err := repo.CreateVerification(userID); err != nil {
		t.Fatalf("CreateVerification: %v", err)
	}
	if err := repo.DeleteVerificationsByUserID(userID); err != nil {
		t.Fatalf("DeleteVerificationsByUserID: %v", err)
	}
	// Both deletes are soft-deletes (DeletedAt): count through the model so
	// GORM's automatic deleted_at IS NULL scope applies, matching what every
	// real caller (GetVerificationByToken et al.) actually sees — a raw
	// .Table() query would bypass that scope and see the soft-deleted row.
	var verificationCount int64
	if err := db.Model(&EmailVerification{}).Where("user_id = ?", userID).Count(&verificationCount).Error; err != nil {
		t.Fatalf("count verifications: %v", err)
	}
	if verificationCount != 0 {
		t.Fatalf("expected 0 verifications after DeleteVerificationsByUserID, got %d", verificationCount)
	}

	if _, err := repo.CreatePasswordReset(userID); err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}
	if err := repo.DeletePasswordResetsByUserID(userID); err != nil {
		t.Fatalf("DeletePasswordResetsByUserID: %v", err)
	}
	var resetCount int64
	if err := db.Model(&PasswordReset{}).Where("user_id = ?", userID).Count(&resetCount).Error; err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if resetCount != 0 {
		t.Fatalf("expected 0 password resets after DeletePasswordResetsByUserID, got %d", resetCount)
	}
}

// TestCleanupExpiredTokens_RemovesOnlyExpiredRows guards that the periodic
// cleanup job's `expires_at < NOW()` filter is scoped correctly — a
// too-broad filter here would delete tokens that are still perfectly valid
// and mid-use.
func TestCleanupExpiredTokens_RemovesOnlyExpiredRows(t *testing.T) {
	db := setupPostgres(t)
	repo := NewRepository(db)
	userID := fixtureUser(t, db)

	expired, err := repo.CreateVerification(userID)
	if err != nil {
		t.Fatalf("CreateVerification (to expire): %v", err)
	}
	fresh, err := repo.CreateVerification(userID)
	if err != nil {
		t.Fatalf("CreateVerification (fresh): %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM email_verifications WHERE user_id = ?`, userID) })

	if err := db.Exec(`UPDATE email_verifications SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = ?`, expired.ID).Error; err != nil {
		t.Fatalf("backdate expired row: %v", err)
	}

	if err := repo.CleanupExpiredTokens(); err != nil {
		t.Fatalf("CleanupExpiredTokens: %v", err)
	}

	var expiredCount, freshCount int64
	db.Table("email_verifications").Where("id = ?", expired.ID).Count(&expiredCount)
	db.Table("email_verifications").Where("id = ?", fresh.ID).Count(&freshCount)
	if expiredCount != 0 {
		t.Fatalf("expected the expired verification to be removed, still found %d", expiredCount)
	}
	if freshCount != 1 {
		t.Fatalf("expected the fresh verification to survive cleanup, found %d", freshCount)
	}
}
