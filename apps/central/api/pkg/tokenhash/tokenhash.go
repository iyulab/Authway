// Package tokenhash provides SHA-256 hashing and secure generation for
// opaque bearer-style tokens (password reset, magic link, admin session,
// etc.) so that only the hash is persisted at rest — a database read never
// yields a usable token.
package tokenhash

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
)

var hashShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// IsHashed reports whether s has the shape of a value Hash could have
// produced (64 lowercase hex characters). It cannot prove s actually is a
// hash of anything in particular — only that it looks like one, which is
// what a backfill needs to skip rows already converted from plaintext
// without re-hashing an already-hashed value into an unrecoverable one.
func IsHashed(s string) bool {
	return hashShape.MatchString(s)
}

// Hash returns the SHA-256 hex digest of a token. It is deterministic — the
// same token always hashes to the same 64-character value — which is what
// makes hash-based lookup (store Hash(token), query by Hash(incoming)) work.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Generate returns a new cryptographically-random, URL-safe token carrying
// 256 bits of entropy. The unpadded base64url encoding is safe to place
// directly in a URL query string or email link without further escaping.
func Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
