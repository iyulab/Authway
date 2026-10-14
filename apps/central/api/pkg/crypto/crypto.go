// Package crypto provides authenticated symmetric encryption for secrets that
// must be recoverable at rest and so cannot be hashed: TOTP shared secrets
// (the server must reproduce the exact secret to validate a code) and the
// credentials a client holds at another identity provider (the server must
// present them there).
//
// The scheme is AES-256-GCM. Ciphertext is stored as:
//
//	"gcm1:" + base64(nonce ‖ ciphertext‖tag)
//
// The "gcm1:" prefix is a scheme tag. Decrypt treats any value WITHOUT the
// prefix as legacy plaintext and returns it verbatim, so a binary rollout can
// precede the pass that seals what is stored.
//
// The key can be changed. A cipher holds the current key and any previous
// ones: it seals with the current key and opens a value with whichever key
// sealed it (GCM's authentication tag rejects every other key, so trying them
// in turn is exact). Reseal turns a stored value into one sealed with the
// current key, so a startup pass can move everything over, after which the
// previous keys can be dropped. The stored form names no key, so a binary
// that predates key rotation reads whatever the current key sealed.
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

// SealedPrefix is what every value sealed by a Cipher starts with — for a
// query that has to tell sealed rows from the rest.
const SealedPrefix = gcmV1Prefix

// keySize is the required AES-256 key length in bytes.
const keySize = 32

// Cipher encrypts and decrypts at-rest secrets.
type Cipher interface {
	// Encrypt returns the storable representation of plaintext.
	Encrypt(plaintext string) (string, error)
	// Decrypt returns the plaintext for a stored value, opened with the
	// current key or a previous one. A value without the scheme prefix is
	// returned verbatim as legacy plaintext.
	Decrypt(stored string) (string, error)
	// Reseal returns stored as it should be stored now — sealed with the
	// current key — and whether that differs from stored: legacy plaintext is
	// sealed, a value sealed with a previous key is opened and sealed again,
	// and a value already sealed with the current key comes back unchanged.
	// A sealed value that no key opens is an error.
	Reseal(stored string) (resealed string, changed bool, err error)
	// Enabled reports whether real encryption is active (a key is configured).
	// When false, Encrypt is a pass-through — used only in development where no
	// key is provisioned.
	Enabled() bool
}

// NewCipher builds a Cipher from a base64-encoded 32-byte key and, while the
// key is being changed, the keys it replaces.
//
//   - keyB64 == ""  → a pass-through cipher (development only; no encryption).
//     Previous keys are refused with it: there is nothing to move them to.
//   - keyB64 set    → AES-256-GCM. Every key MUST base64-decode to exactly
//     32 bytes, else an error is returned (fail-closed at startup).
func NewCipher(keyB64 string, previousB64 ...string) (Cipher, error) {
	if keyB64 == "" {
		if len(previousB64) > 0 {
			return nil, fmt.Errorf("crypto: previous keys given without a current key")
		}
		return passthrough{}, nil
	}

	current, err := newAEAD(keyB64)
	if err != nil {
		return nil, err
	}
	c := &aesGCM{current: current}
	for i, k := range previousB64 {
		if k == keyB64 {
			return nil, fmt.Errorf("crypto: previous key %d is the current key", i+1)
		}
		aead, err := newAEAD(k)
		if err != nil {
			return nil, fmt.Errorf("crypto: previous key %d: %w", i+1, err)
		}
		c.previous = append(c.previous, aead)
	}
	return c, nil
}

func newAEAD(keyB64 string) (cipher.AEAD, error) {
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
	return gcm, nil
}

// aesGCM is the real AES-256-GCM implementation: it seals with the current
// key and opens with the current key or a previous one.
type aesGCM struct {
	current  cipher.AEAD
	previous []cipher.AEAD
}

func (a *aesGCM) Enabled() bool { return true }

func (a *aesGCM) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, a.current.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: failed to read nonce: %w", err)
	}
	sealed := a.current.Seal(nonce, nonce, []byte(plaintext), nil)
	return gcmV1Prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (a *aesGCM) Decrypt(stored string) (string, error) {
	plaintext, _, err := a.open(stored)
	return plaintext, err
}

func (a *aesGCM) Reseal(stored string) (string, bool, error) {
	plaintext, withCurrent, err := a.open(stored)
	if err != nil {
		return "", false, err
	}
	if withCurrent {
		return stored, false, nil
	}
	resealed, err := a.Encrypt(plaintext)
	if err != nil {
		return "", false, err
	}
	return resealed, true, nil
}

// open returns the plaintext of a stored value and whether the current key
// sealed it. Legacy plaintext (no prefix) comes back verbatim, not sealed
// with the current key.
func (a *aesGCM) open(stored string) (string, bool, error) {
	if !strings.HasPrefix(stored, gcmV1Prefix) {
		return stored, false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, gcmV1Prefix))
	if err != nil {
		return "", false, fmt.Errorf("crypto: ciphertext is not valid base64: %w", err)
	}
	nonceSize := a.current.NonceSize()
	if len(raw) < nonceSize {
		return "", false, fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	if plaintext, err := a.current.Open(nil, nonce, ct, nil); err == nil {
		return string(plaintext), true, nil
	}
	for _, aead := range a.previous {
		if plaintext, err := aead.Open(nil, nonce, ct, nil); err == nil {
			return string(plaintext), false, nil
		}
	}
	return "", false, fmt.Errorf("crypto: decryption failed (sealed with a key that is not configured, or corrupt data)")
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

func (p passthrough) Reseal(stored string) (string, bool, error) {
	if _, err := p.Decrypt(stored); err != nil {
		return "", false, err
	}
	return stored, false, nil
}

// IsEncrypted reports whether a stored value carries the scheme prefix.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, gcmV1Prefix)
}
