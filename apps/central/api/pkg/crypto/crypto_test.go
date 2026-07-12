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
