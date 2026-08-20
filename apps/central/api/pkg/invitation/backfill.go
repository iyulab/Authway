package invitation

import (
	"fmt"

	"authway/apps/central/api/pkg/tokenhash"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BackfillTokenHashes re-hashes any legacy plaintext invitation tokens in
// place.
//
// Migration 020 renamed invitations.token to token_hash but could not hash
// existing rows in SQL — pgcrypto is not allow-listed on this Azure Database
// for PostgreSQL instance — so a row created before that migration deployed
// still holds its plaintext token in the (now misleadingly-named) column
// until this runs.
//
// It is idempotent: a value that already looks like a SHA-256 hex digest is
// left alone, so it is safe to run on every startup alongside schema
// migrations. Unlike migration 019 (magic links) invalidating every
// outstanding token outright, this preserves any invitation issued before
// the migration — hashing the plaintext already in the column reproduces
// exactly the hash GetByToken computes from the same plaintext arriving in
// an email link, so a pending invitation stays redeemable across the
// upgrade.
//
// Concurrency: the write is conditional on the plaintext still being present
// (WHERE token_hash = <read value>). If a row is re-issued (Resend) mid
// backfill and stores a fresh hash, the stale write matches 0 rows and is
// skipped — preventing a lost-update that would roll the token back to a
// value nobody was emailed.
func BackfillTokenHashes(db *gorm.DB, logger *zap.Logger) error {
	var invitations []Invitation
	if err := db.Model(&Invitation{}).Find(&invitations).Error; err != nil {
		return fmt.Errorf("backfill: failed to load invitations: %w", err)
	}

	migrated, failed := 0, 0
	for _, inv := range invitations {
		if tokenhash.IsHashed(inv.TokenHash) {
			continue
		}
		hash := tokenhash.Hash(inv.TokenHash)
		res := db.Model(&Invitation{}).
			Where("id = ? AND token_hash = ?", inv.ID, inv.TokenHash).
			Update("token_hash", hash)
		if res.Error != nil {
			logger.Error("backfill: failed to hash invitation token",
				zap.String("invitation_id", inv.ID.String()), zap.Error(res.Error))
			failed++
			continue
		}
		if res.RowsAffected == 1 {
			migrated++
		}
	}

	if migrated > 0 || failed > 0 {
		logger.Info("Backfilled invitation token hashing",
			zap.Int("migrated", migrated), zap.Int("failed", failed))
	}
	return nil
}
