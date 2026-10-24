package client

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// hashSecret returns what is stored for a client secret. A secret may be one
// the caller chose, so it gets a password hash; it is digested first because
// bcrypt reads at most 72 bytes.
func hashSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(secretDigest(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash the client secret: %w", err)
	}
	return string(hash), nil
}

func secretDigest(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return []byte(base64.RawStdEncoding.EncodeToString(sum[:]))
}

// secretMatches reports whether secret is the client's.
func (c *Client) secretMatches(secret string) bool {
	if secret == "" || c.ClientSecretHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(c.ClientSecretHash), secretDigest(secret)) == nil
}
