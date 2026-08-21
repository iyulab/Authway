package mfa

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors pkg/invitation's helper of the same name — a real
// Postgres is required because BackfillTOTPSecrets and the encryption scheme
// it converges (pkg/crypto's "gcm1:" prefix) are exercised against the real
// users table brought up by the migrator, not an in-memory substitute.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres mfa tests")
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

// testCipher returns a real AES-256-GCM cipher backed by a fresh random key —
// exercising the same encrypt/decrypt path production uses, not a stub.
func testCipher(t *testing.T) crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	c, err := crypto.NewCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return c
}

func freshTenant(t *testing.T, ts *tenant.Service) *tenant.Tenant {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{
		Name: "mfa-test-" + suffix,
		Slug: "mfa-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tn
}

func newTestUser(t *testing.T, db *gorm.DB, us user.Service, tenantID uuid.UUID) *user.User {
	t.Helper()
	email := fmt.Sprintf("mfa-%s@example.com", uuid.New().String()[:8])
	u, err := us.Create(tenantID, &user.CreateUserRequest{Email: email, Name: "MFA Test User", Password: "correct-horse-battery-staple"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	return u
}

// newTestService wires the real service + a real (non-passthrough) cipher —
// SetupTOTP/VerifyAndEnable/Verify all round-trip through Encrypt/Decrypt, so
// a passthrough cipher would validate even a broken encryption integration.
func newTestService(t *testing.T, db *gorm.DB) (Service, user.Service, *tenant.Service) {
	t.Helper()
	us := user.NewService(db, zap.NewNop())
	ts := tenant.NewService(db)
	svc := NewService(db, us, zap.NewNop(), "Authway Test", testCipher(t))
	return svc, us, ts
}

func currentCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	return code
}

// TestMFALifecycle_SetupVerifyEnableDisable exercises the full happy path a
// real login flow drives: setup issues an encrypted-at-rest secret, a valid
// code enables MFA and mints recovery codes, Verify accepts a fresh code and
// rejects garbage, and Disable clears every MFA-related column.
func TestMFALifecycle_SetupVerifyEnableDisable(t *testing.T) {
	db := setupPostgres(t)
	svc, us, ts := newTestService(t, db)
	tn := freshTenant(t, ts)
	u := newTestUser(t, db, us, tn.ID)

	setup, err := svc.SetupTOTP(u.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if setup.Secret == "" {
		t.Fatal("expected a non-empty TOTP secret")
	}

	// The column must hold the encrypted form, never the plaintext secret
	// SetupTOTP returned to the caller.
	var stored string
	if err := db.Raw(`SELECT totp_secret FROM users WHERE id = ?`, u.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read back totp_secret: %v", err)
	}
	if !crypto.IsEncrypted(stored) {
		t.Fatalf("expected totp_secret to be stored encrypted, got %q", stored)
	}
	if stored == setup.Secret {
		t.Fatal("totp_secret column must not equal the plaintext secret")
	}

	status, err := svc.GetStatus(u.ID)
	if err != nil {
		t.Fatalf("GetStatus (pre-enable): %v", err)
	}
	if status.Enabled {
		t.Fatal("expected MFA not yet enabled before VerifyAndEnable")
	}

	if _, err := svc.VerifyAndEnable(u.ID, "000000"); err == nil {
		t.Fatal("expected VerifyAndEnable to reject a wrong code")
	}

	recovery, err := svc.VerifyAndEnable(u.ID, currentCode(t, setup.Secret))
	if err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}
	if len(recovery.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(recovery.RecoveryCodes))
	}

	status, err = svc.GetStatus(u.ID)
	if err != nil {
		t.Fatalf("GetStatus (post-enable): %v", err)
	}
	if !status.Enabled {
		t.Fatal("expected MFA enabled after VerifyAndEnable")
	}
	if status.RecoveryCodesLeft != 8 {
		t.Fatalf("expected 8 recovery codes left, got %d", status.RecoveryCodesLeft)
	}

	ok, err := svc.Verify(u.ID, currentCode(t, setup.Secret))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to accept a freshly generated code")
	}

	ok, err = svc.Verify(u.ID, "000000")
	if err != nil {
		t.Fatalf("Verify (wrong code): %v", err)
	}
	if ok {
		t.Fatal("expected Verify to reject a wrong code")
	}

	if err := svc.Disable(u.ID); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	status, err = svc.GetStatus(u.ID)
	if err != nil {
		t.Fatalf("GetStatus (post-disable): %v", err)
	}
	if status.Enabled {
		t.Fatal("expected MFA disabled after Disable")
	}

	var secretAfterDisable *string
	if err := db.Raw(`SELECT totp_secret FROM users WHERE id = ?`, u.ID).Scan(&secretAfterDisable).Error; err != nil {
		t.Fatalf("read back totp_secret after disable: %v", err)
	}
	if secretAfterDisable != nil {
		t.Fatalf("expected totp_secret to be cleared after Disable, got %v", *secretAfterDisable)
	}
}

// TestMFARecoveryCodes_ConsumedOnceAndRegenerable guards two properties the
// handler layer depends on: a recovery code works exactly once, and
// regeneration fully replaces the set (an old code stops working).
func TestMFARecoveryCodes_ConsumedOnceAndRegenerable(t *testing.T) {
	db := setupPostgres(t)
	svc, us, ts := newTestService(t, db)
	tn := freshTenant(t, ts)
	u := newTestUser(t, db, us, tn.ID)

	setup, err := svc.SetupTOTP(u.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	recovery, err := svc.VerifyAndEnable(u.ID, currentCode(t, setup.Secret))
	if err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}
	code := recovery.RecoveryCodes[0]

	ok, err := svc.VerifyRecoveryCode(u.ID, code)
	if err != nil {
		t.Fatalf("VerifyRecoveryCode (first use): %v", err)
	}
	if !ok {
		t.Fatal("expected first use of a fresh recovery code to succeed")
	}

	ok, err = svc.VerifyRecoveryCode(u.ID, code)
	if err != nil {
		t.Fatalf("VerifyRecoveryCode (reuse): %v", err)
	}
	if ok {
		t.Fatal("expected a recovery code to be rejected on reuse")
	}

	regenerated, err := svc.RegenerateRecoveryCodes(u.ID)
	if err != nil {
		t.Fatalf("RegenerateRecoveryCodes: %v", err)
	}

	// An untouched code from the original batch must stop working once the
	// set has been regenerated.
	staleCode := recovery.RecoveryCodes[1]
	ok, err = svc.VerifyRecoveryCode(u.ID, staleCode)
	if err != nil {
		t.Fatalf("VerifyRecoveryCode (stale, post-regenerate): %v", err)
	}
	if ok {
		t.Fatal("expected a pre-regeneration recovery code to be invalid after RegenerateRecoveryCodes")
	}

	ok, err = svc.VerifyRecoveryCode(u.ID, regenerated.RecoveryCodes[0])
	if err != nil {
		t.Fatalf("VerifyRecoveryCode (new code): %v", err)
	}
	if !ok {
		t.Fatal("expected a freshly regenerated recovery code to work")
	}
}
