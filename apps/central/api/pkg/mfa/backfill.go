package mfa

import (
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/user"
)

// BackfillTOTPSecrets re-encrypts any legacy plaintext TOTP secrets in place.
//
// It is idempotent: rows already carrying the scheme prefix are skipped, so it
// is safe to run on every startup alongside schema migrations. It is a no-op
// when the cipher has no key configured (Enabled() == false) — without a key
// there is nothing to encrypt to, and the lazy pass-through in Decrypt keeps
// legacy plaintext working regardless.
//
// Because the lazy pass-through makes validation work whether or not this runs,
// the backfill is a non-critical convergence step: a per-row failure is logged
// and skipped rather than aborting, and the caller must NOT treat a returned
// error as fatal (an unavailable IdP is worse than a still-plaintext secret).
//
// Concurrency: the write is conditional on the plaintext still being present
// (WHERE totp_secret = <read value>). If a user re-runs SetupTOTP mid-backfill
// and stores a fresh secret, the stale write matches 0 rows and is skipped —
// preventing a lost-update that would silently roll their secret back.
//
// NOTE: whether any plaintext rows actually exist in a given environment is not
// known at build time (MFA setup is exposed but not enforced). This backfill is
// written defensively so it converges correctly whether that count is zero or
// not, without a separate SQL migration (the totp_secret column is already TEXT
// and needs no schema change).
func BackfillTOTPSecrets(db *gorm.DB, c crypto.Cipher, logger *zap.Logger) error {
	if !c.Enabled() {
		return nil
	}

	var users []user.User
	if err := db.Model(&user.User{}).
		Where("totp_secret IS NOT NULL AND totp_secret <> ''").
		Find(&users).Error; err != nil {
		return fmt.Errorf("backfill: failed to load users with TOTP secrets: %w", err)
	}

	migrated, failed := 0, 0
	for _, u := range users {
		if u.TOTPSecret == nil || crypto.IsEncrypted(*u.TOTPSecret) {
			continue
		}
		enc, err := c.Encrypt(*u.TOTPSecret)
		if err != nil {
			logger.Error("backfill: failed to encrypt TOTP secret",
				zap.String("user_id", u.ID.String()), zap.Error(err))
			failed++
			continue
		}
		// Optimistic write: only replace the row if the plaintext we read is
		// still there. A concurrent SetupTOTP wins → RowsAffected == 0, skipped.
		res := db.Model(&user.User{}).
			Where("id = ? AND totp_secret = ?", u.ID, *u.TOTPSecret).
			Update("totp_secret", enc)
		if res.Error != nil {
			logger.Error("backfill: failed to store encrypted TOTP secret",
				zap.String("user_id", u.ID.String()), zap.Error(res.Error))
			failed++
			continue
		}
		if res.RowsAffected == 1 {
			migrated++
		}
	}

	if migrated > 0 || failed > 0 {
		logger.Info("Backfilled TOTP secret encryption",
			zap.Int("migrated", migrated), zap.Int("failed", failed))
	}
	return nil
}
