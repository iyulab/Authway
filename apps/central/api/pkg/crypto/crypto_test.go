package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func newTestKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	c, err := NewCipher(newTestKey(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	if !c.Enabled() {
		t.Fatal("expected cipher to be enabled with a key")
	}

	plaintext := "JBSWY3DPEHPK3PXP" // representative base32 TOTP secret
	enc, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("ciphertext missing scheme prefix: %q", enc)
	}
	if enc == plaintext {
		t.Fatal("ciphertext equals plaintext")
	}

	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plaintext {
		t.Fatalf("roundtrip mismatch: got %q want %q", dec, plaintext)
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	a, _ := c.Encrypt("same-secret")
	b, _ := c.Encrypt("same-secret")
	if a == b {
		t.Fatal("expected distinct ciphertexts (random nonce) for identical plaintext")
	}
}

func TestDecryptLegacyPlaintextPassesThrough(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	// A pre-encryption row (no prefix) must be returned verbatim so validation
	// keeps working before the backfill runs.
	legacy := "JBSWY3DPEHPK3PXP"
	got, err := c.Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt legacy: %v", err)
	}
	if got != legacy {
		t.Fatalf("legacy passthrough mismatch: got %q want %q", got, legacy)
	}
}

func TestBackfillIdempotency(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	enc, _ := c.Encrypt("secret")
	// IsEncrypted gates the backfill skip — an already-encrypted value must be
	// detected so re-running does not double-encrypt.
	if !IsEncrypted(enc) {
		t.Fatal("expected already-encrypted value to be detected")
	}
	// And decrypting it once more still yields the original (not re-wrapped).
	dec, err := c.Decrypt(enc)
	if err != nil || dec != "secret" {
		t.Fatalf("double-decrypt guard: got %q err %v", dec, err)
	}
}

func TestWrongKeyFailsDecryption(t *testing.T) {
	c1, _ := NewCipher(newTestKey(t))
	c2, _ := NewCipher(newTestKey(t))
	enc, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("expected decryption with wrong key to fail (GCM auth)")
	}
}

func TestPassthroughWhenNoKey(t *testing.T) {
	c, err := NewCipher("")
	if err != nil {
		t.Fatalf("NewCipher(\"\"): %v", err)
	}
	if c.Enabled() {
		t.Fatal("expected passthrough cipher to report disabled")
	}
	enc, _ := c.Encrypt("secret")
	if enc != "secret" {
		t.Fatalf("passthrough Encrypt altered value: %q", enc)
	}
	// A prefixed value with no key is a misconfiguration → error.
	if _, err := c.Decrypt(gcmV1Prefix + "abc"); err == nil {
		t.Fatal("expected error decrypting prefixed value without a key")
	}
}

func TestNewCipherRejectsBadKeys(t *testing.T) {
	if _, err := NewCipher("not-base64!!!"); err == nil {
		t.Fatal("expected error for non-base64 key")
	}
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := NewCipher(short); err == nil {
		t.Fatal("expected error for 16-byte key (not AES-256)")
	}
}

// Changing the key: a cipher given the old key as a previous key opens what
// the old key sealed, reseals it with the new key, and leaves alone what the
// new key already sealed. After that, the new key alone reads everything.
func TestPreviousKeyOpensWhatItSealedAndResealMovesItToTheCurrentKey(t *testing.T) {
	oldKey, newKey := newTestKey(t), newTestKey(t)
	before, _ := NewCipher(oldKey)
	sealedWithOld, _ := before.Encrypt("secret")

	ring, err := NewCipher(newKey, oldKey)
	if err != nil {
		t.Fatalf("NewCipher with a previous key: %v", err)
	}
	if got, err := ring.Decrypt(sealedWithOld); err != nil || got != "secret" {
		t.Fatalf("Decrypt of a value the previous key sealed: got %q err %v", got, err)
	}

	resealed, changed, err := ring.Reseal(sealedWithOld)
	if err != nil || !changed {
		t.Fatalf("Reseal of a value the previous key sealed: changed=%v err=%v", changed, err)
	}
	again, changed, err := ring.Reseal(resealed)
	if err != nil || changed || again != resealed {
		t.Fatalf("Reseal of a value the current key sealed: changed=%v err=%v", changed, err)
	}

	after, _ := NewCipher(newKey)
	if got, err := after.Decrypt(resealed); err != nil || got != "secret" {
		t.Fatalf("the current key alone does not read the resealed value: got %q err %v", got, err)
	}
	// And the old key no longer reads it — what it guarded is now under the
	// new key only.
	if _, err := before.Decrypt(resealed); err == nil {
		t.Fatal("the old key still opens the resealed value")
	}
}

func TestResealSealsPlaintext(t *testing.T) {
	c, _ := NewCipher(newTestKey(t))
	resealed, changed, err := c.Reseal("JBSWY3DPEHPK3PXP")
	if err != nil || !changed || !IsEncrypted(resealed) {
		t.Fatalf("Reseal of plaintext: %q changed=%v err=%v", resealed, changed, err)
	}
	if got, _ := c.Decrypt(resealed); got != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("resealed plaintext reads %q", got)
	}
}

// A value sealed with a key that is neither current nor previous is an
// error, not something to pass on: dropping a previous key too early shows
// here.
func TestResealFailsForAKeyThatIsNotConfigured(t *testing.T) {
	other, _ := NewCipher(newTestKey(t))
	sealed, _ := other.Encrypt("secret")
	ring, _ := NewCipher(newTestKey(t), newTestKey(t))
	if _, _, err := ring.Reseal(sealed); err == nil {
		t.Fatal("expected Reseal to fail for a value no configured key sealed")
	}
	if _, err := ring.Decrypt(sealed); err == nil {
		t.Fatal("expected Decrypt to fail for a value no configured key sealed")
	}
}

func TestNewCipherRejectsBadPreviousKeys(t *testing.T) {
	current := newTestKey(t)
	if _, err := NewCipher("", newTestKey(t)); err == nil {
		t.Fatal("expected previous keys without a current key to be refused")
	}
	if _, err := NewCipher(current, current); err == nil {
		t.Fatal("expected the current key repeated as a previous key to be refused")
	}
	if _, err := NewCipher(current, base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Fatal("expected a 16-byte previous key to be refused")
	}
}
