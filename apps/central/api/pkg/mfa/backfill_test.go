package mfa

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"go.uber.org/zap"

	"authway/apps/central/api/pkg/crypto"
)

// TestResealTOTPSecrets_EncryptsLegacyPlaintextInPlace is the key regression
// guard for the design pkg/crypto documents: a legacy plaintext secret must
// still validate TOTP codes correctly after the backfill re-encrypts it —
// mirrors pkg/invitation's TestBackfillTokenHashes_PreservesRedeemability for
// the same reason (in-place convergence must not break the value it converts).
func TestResealTOTPSecrets_EncryptsLegacyPlaintextInPlace(t *testing.T) {
	db := setupPostgres(t)
	svc, us, ts := newTestService(t, db)
	tn := freshTenant(t, ts)
	u := newTestUser(t, db, us, tn.ID)

	// Seed a legacy plaintext secret directly, bypassing SetupTOTP — this is
	// the pre-encryption-rollout state the backfill exists to converge.
	plaintextSecret := "JBSWY3DPEHPK3PXP"
	if err := db.Exec(`UPDATE users SET totp_secret = ?, totp_enabled = true WHERE id = ?`, plaintextSecret, u.ID).Error; err != nil {
		t.Fatalf("seed legacy plaintext secret: %v", err)
	}

	// Before backfill: Verify must still work against the plaintext column
	// (the lazy pass-through in Decrypt), proving the migration is a
	// convergence step and not a prerequisite for correctness.
	ok, err := svc.Verify(u.ID, currentCode(t, plaintextSecret))
	if err != nil {
		t.Fatalf("Verify (pre-backfill): %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to accept a valid code against a legacy plaintext secret")
	}

	cipher := testCipher(t)
	if err := ResealTOTPSecrets(db, cipher, zap.NewNop()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var stored string
	if err := db.Raw(`SELECT totp_secret FROM users WHERE id = ?`, u.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read back totp_secret: %v", err)
	}
	if !crypto.IsEncrypted(stored) {
		t.Fatalf("expected totp_secret to carry the encryption scheme prefix after backfill, got %q", stored)
	}

	// The service must be reconstructed with the SAME cipher the backfill
	// used — Verify still needs to decrypt what was just encrypted.
	svcWithSameCipher := onTestClock(NewService(db, us, zap.NewNop(), "Authway Test", cipher))
	ok, err = svcWithSameCipher.Verify(u.ID, currentCode(t, plaintextSecret))
	if err != nil {
		t.Fatalf("Verify (post-backfill): %v", err)
	}
	if !ok {
		t.Fatal("expected the original plaintext secret to still validate codes after backfill re-encrypts it")
	}
}

// TestResealTOTPSecrets_IdempotentOnAlreadyEncryptedRows guards against the
// backfill re-encrypting a value that already carries the scheme prefix,
// which would double-wrap it and make every subsequent Decrypt fail (Decrypt
// only ever strips one prefix layer).
func TestResealTOTPSecrets_IdempotentOnAlreadyEncryptedRows(t *testing.T) {
	db := setupPostgres(t)
	svc, us, ts := newTestService(t, db)
	tn := freshTenant(t, ts)
	u := newTestUser(t, db, us, tn.ID)

	setup, err := svc.SetupTOTP(u.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if _, err := svc.VerifyAndEnable(u.ID, currentCode(t, setup.Secret)); err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}

	if err := ResealTOTPSecrets(db, testCipher(t), zap.NewNop()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// A normally-enrolled secret must still validate after a backfill run —
	// if the already-encrypted row had been re-wrapped, this would fail.
	ok, err := svc.Verify(u.ID, currentCode(t, setup.Secret))
	if err != nil {
		t.Fatalf("Verify (post-backfill, already-encrypted row): %v", err)
	}
	if !ok {
		t.Fatal("expected a normally-enrolled secret to remain valid after a redundant backfill run")
	}
}

// TestResealTOTPSecrets_NoOpWhenCipherDisabled guards the documented
// behavior in backfill.go: with no encryption key configured there is
// nothing to encrypt to, so the backfill must leave rows untouched rather
// than erroring or blocking startup.
func TestResealTOTPSecrets_NoOpWhenCipherDisabled(t *testing.T) {
	db := setupPostgres(t)
	_, us, ts := newTestService(t, db)
	tn := freshTenant(t, ts)
	u := newTestUser(t, db, us, tn.ID)

	plaintextSecret := "JBSWY3DPEHPK3PXP"
	if err := db.Exec(`UPDATE users SET totp_secret = ? WHERE id = ?`, plaintextSecret, u.ID).Error; err != nil {
		t.Fatalf("seed legacy plaintext secret: %v", err)
	}

	passthroughCipher, err := crypto.NewCipher("")
	if err != nil {
		t.Fatalf("new passthrough cipher: %v", err)
	}
	if passthroughCipher.Enabled() {
		t.Fatal("expected an empty key to produce a disabled (passthrough) cipher")
	}

	if err := ResealTOTPSecrets(db, passthroughCipher, zap.NewNop()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var stored string
	if err := db.Raw(`SELECT totp_secret FROM users WHERE id = ?`, u.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read back totp_secret: %v", err)
	}
	if stored != plaintextSecret {
		t.Fatalf("expected totp_secret to be left untouched when the cipher is disabled, got %q", stored)
	}
}

// Changing the at-rest key: a secret sealed with the old key is resealed with
// the new one, and afterwards a service holding only the new key still
// verifies the user's codes — the user notices nothing.
func TestResealTOTPSecrets_MovesSecretsFromAPreviousKey(t *testing.T) {
	db := setupPostgres(t)
	_, us, ts := newTestService(t, db)
	tn := freshTenant(t, ts)
	u := newTestUser(t, db, us, tn.ID)

	oldKey, newKey := newKeyB64(t), newKeyB64(t)
	oldCipher, _ := crypto.NewCipher(oldKey)
	withOld := onTestClock(NewService(db, us, zap.NewNop(), "Authway Test", oldCipher))
	setup, err := withOld.SetupTOTP(u.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if _, err := withOld.VerifyAndEnable(u.ID, currentCode(t, setup.Secret)); err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}
	var sealedWithOld string
	db.Raw(`SELECT totp_secret FROM users WHERE id = ?`, u.ID).Scan(&sealedWithOld)

	ring, err := crypto.NewCipher(newKey, oldKey)
	if err != nil {
		t.Fatalf("cipher with a previous key: %v", err)
	}
	if err := ResealTOTPSecrets(db, ring, zap.NewNop()); err != nil {
		t.Fatalf("reseal: %v", err)
	}
	var resealed string
	db.Raw(`SELECT totp_secret FROM users WHERE id = ?`, u.ID).Scan(&resealed)
	if resealed == sealedWithOld || !crypto.IsEncrypted(resealed) {
		t.Fatalf("expected the secret to be resealed, stored value unchanged=%v sealed=%v", resealed == sealedWithOld, crypto.IsEncrypted(resealed))
	}

	newOnly, _ := crypto.NewCipher(newKey)
	withNew := onTestClock(NewService(db, us, zap.NewNop(), "Authway Test", newOnly))
	ok, err := withNew.Verify(u.ID, currentCode(t, setup.Secret))
	if err != nil || !ok {
		t.Fatalf("a service with only the new key does not verify the user's code: ok=%v err=%v", ok, err)
	}
}

func newKeyB64(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}
