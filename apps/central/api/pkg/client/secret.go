package client

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"

	"go.uber.org/zap"
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

// secretMatches reports whether secret is the client's. A row written before
// secrets were hashed still holds the secret itself; it is compared in
// constant time.
func (c *Client) secretMatches(secret string) bool {
	if secret == "" {
		return false
	}
	if c.ClientSecretHash != "" {
		return bcrypt.CompareHashAndPassword([]byte(c.ClientSecretHash), secretDigest(secret)) == nil
	}
	return c.ClientSecret != "" && subtle.ConstantTimeCompare([]byte(c.ClientSecret), []byte(secret)) == 1
}

// BackfillSecretHashes gives every client that still has only its secret a
// hash of it, so the secret can be checked the same way for every client. It
// leaves the stored secret in place: removing it is a separate, deliberate
// step, since nothing can bring a secret back.
func BackfillSecretHashes(svc Service, logger *zap.Logger) error {
	s, ok := svc.(*service)
	if !ok {
		return nil
	}
	var pending []Client
	// Deleted clients are included: their rows hold a secret all the same.
	if err := s.db.Unscoped().Where("client_secret <> '' AND client_secret_hash = ''").Find(&pending).Error; err != nil {
		return fmt.Errorf("failed to list clients without a secret hash: %w", err)
	}
	for i := range pending {
		hash, err := hashSecret(pending[i].ClientSecret)
		if err != nil {
			return err
		}
		if err := s.db.Unscoped().Model(&Client{}).Where("id = ? AND client_secret_hash = ''", pending[i].ID).
			Update("client_secret_hash", hash).Error; err != nil {
			return fmt.Errorf("failed to store a secret hash: %w", err)
		}
	}
	if len(pending) > 0 {
		logger.Info("Hashed client secrets that were stored as they are", zap.Int("clients", len(pending)))
	}
	return nil
}
