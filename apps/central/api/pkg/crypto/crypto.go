// Package crypto provides authenticated symmetric encryption for secrets that
// must be recoverable at rest — currently TOTP shared secrets, which cannot be
// hashed (the server must reproduce the exact secret to validate a code).
//
// The scheme is AES-256-GCM. Ciphertext is stored as:
//
//	"gcm1:" + base64(nonce ‖ ciphertext‖tag)
//
// The "gcm1:" prefix is a scheme tag. Decrypt treats any value WITHOUT the
// prefix as legacy plaintext and returns it verbatim, so three things stay
// non-fragile: a binary rollout that precedes the backfill, a backfill that is
// idempotent (already-prefixed rows are skipped), and a future key rotation
// (the tag can carry a new version).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// gcmV1Prefix tags a value as AES-256-GCM (scheme v1) encrypted. A value
// lacking this prefix is legacy plaintext.
const gcmV1Prefix = "gcm1:"

// keySize is the required AES-256 key length in bytes.
const keySize = 32

// Cipher encrypts and decrypts at-rest secrets.
type Cipher interface {
	// Encrypt returns the storable representation of plaintext.
	Encrypt(plaintext string) (string, error)
	// Decrypt returns the plaintext for a stored value. A value without the
	// scheme prefix is returned verbatim as legacy plaintext.
	Decrypt(stored string) (string, error)
	// Enabled reports whether real encryption is active (a key is configured).
	// When false, Encrypt is a pass-through — used only in development where no
	// key is provisioned.
	Enabled() bool
}

// NewCipher builds a Cipher from a base64-encoded 32-byte key.
//
//   - keyB64 == ""  → a pass-through cipher (development only; no encryption).
//   - keyB64 set    → AES-256-GCM, and the key MUST base64-decode to exactly
//     32 bytes, else an error is returned (fail-closed at startup).
func NewCipher(keyB64 string) (Cipher, error) {
	if keyB64 == "" {
		return passthrough{}, nil
	}

	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: key is not valid base64: %w", err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("crypto: key must decode to %d bytes (AES-256), got %d", keySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to create AES block: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to create GCM: %w", err)
	}
	return &aesGCM{gcm: gcm}, nil
}

// aesGCM is the real AES-256-GCM implementation.
type aesGCM struct {
	gcm cipher.AEAD
}

func (a *aesGCM) Enabled() bool { return true }

func (a *aesGCM) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: failed to read nonce: %w", err)
	}
	sealed := a.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return gcmV1Prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (a *aesGCM) Decrypt(stored string) (string, error) {
	// Legacy plaintext (pre-encryption rows, or a backfill not yet run) has no
	// prefix — return it verbatim so validation keeps working.
	if !strings.HasPrefix(stored, gcmV1Prefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, gcmV1Prefix))
	if err != nil {
		return "", fmt.Errorf("crypto: ciphertext is not valid base64: %w", err)
	}
	nonceSize := a.gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := a.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decryption failed (wrong key or corrupt data): %w", err)
	}
	return string(plaintext), nil
}

// passthrough is the no-key development fallback: values are stored as-is.
type passthrough struct{}

func (passthrough) Enabled() bool { return false }

func (passthrough) Encrypt(plaintext string) (string, error) { return plaintext, nil }

func (passthrough) Decrypt(stored string) (string, error) {
	// A prefixed value cannot be decrypted without a key — that means the key
	// was removed after data was encrypted, which is a misconfiguration.
	if strings.HasPrefix(stored, gcmV1Prefix) {
		return "", fmt.Errorf("crypto: encrypted value present but no key configured (AUTHWAY_TOTP_ENCRYPTION_KEY missing)")
	}
	return stored, nil
}

// IsEncrypted reports whether a stored value carries the scheme prefix. Used by
// the backfill to skip already-encrypted rows (idempotency).
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, gcmV1Prefix)
}
