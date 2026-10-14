package mfa

import (
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/user"
)

// ResealTOTPSecrets stores every TOTP secret sealed with the current at-rest
// key: a secret stored in the clear (from before sealing, or while no key was
// configured) is sealed, and one sealed with a previous key is opened and
// sealed again. A secret already sealed with the current key is left alone,
// so this runs on every startup alongside the schema migrations. Once it has
// run with nothing left over, the previous keys can be removed.
//
// It is a no-op when the cipher has no key configured (Enabled() == false) —
// there is nothing to seal to, and Decrypt reads plaintext regardless.
//
// It converges; it is not a precondition: Decrypt reads plaintext and opens
// values sealed with any configured key, so validation works whether or not
// this has run. A per-row failure is logged and skipped rather than aborting,
// and the caller must NOT treat a returned error as fatal (an unavailable IdP
// is worse than a secret not yet moved to the current key).
//
// Concurrency: the write is conditional on the stored value still being what
// was read (WHERE totp_secret = <read value>). If a user re-runs SetupTOTP
// meanwhile and stores a fresh secret, the stale write matches 0 rows and is
// skipped — preventing a lost update that would roll their secret back.
func ResealTOTPSecrets(db *gorm.DB, c crypto.Cipher, logger *zap.Logger) error {
	if !c.Enabled() {
		return nil
	}

	// Deleted users are included: their rows hold the secret all the same.
	var users []user.User
	if err := db.Unscoped().Model(&user.User{}).
		Where("totp_secret IS NOT NULL AND totp_secret <> ''").
		Find(&users).Error; err != nil {
		return fmt.Errorf("reseal: failed to load users with TOTP secrets: %w", err)
	}

	resealed, unreadable, failed := 0, 0, 0
	for _, u := range users {
		if u.TOTPSecret == nil {
			continue
		}
		value, changed, err := c.Reseal(*u.TOTPSecret)
		if err != nil {
			// Sealed with a key that is not configured: it stays unreadable
			// until that key is given back as a previous key.
			logger.Error("reseal: a TOTP secret is not readable with any configured key",
				zap.String("user_id", u.ID.String()), zap.Error(err))
			unreadable++
			continue
		}
		if !changed {
			continue
		}
		res := db.Unscoped().Model(&user.User{}).
			Where("id = ? AND totp_secret = ?", u.ID, *u.TOTPSecret).
			Update("totp_secret", value)
		if res.Error != nil {
			logger.Error("reseal: failed to store a resealed TOTP secret",
				zap.String("user_id", u.ID.String()), zap.Error(res.Error))
			failed++
			continue
		}
		if res.RowsAffected == 1 {
			resealed++
		}
	}

	if resealed > 0 || unreadable > 0 || failed > 0 {
		logger.Info("Resealed TOTP secrets with the current at-rest key",
			zap.Int("resealed", resealed), zap.Int("unreadable", unreadable), zap.Int("failed", failed))
	}
	return nil
}
