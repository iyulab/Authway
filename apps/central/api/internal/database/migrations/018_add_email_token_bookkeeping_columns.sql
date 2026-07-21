-- ============================================================
-- Migration 018: Email Token Bookkeeping Columns
-- ============================================================
-- The GORM models for password_resets (UsedAt, UpdatedAt) and
-- email_verifications (UpdatedAt) declare columns that no migration ever
-- created — migration 000 predates them and 013/014 only renamed
-- token -> token_hash. Every write through those models therefore failed
-- with SQLSTATE 42703 ("column updated_at does not exist"): the
-- forgot-password and resend-verification flows have never worked against
-- the migrated schema.
--
-- Same defect class as migration 016 (invitations): service tests build
-- their schema with AutoMigrate from the same struct, so they cannot see
-- model<->schema drift by construction. The schema contract test now
-- covers both models.
--
-- Guarded with IF NOT EXISTS so the file is safe on databases that were
-- ever provisioned via AutoMigrate (dev) as well as migrated ones.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee. Migration files must contain no
-- transaction-control statements.
-- ============================================================

ALTER TABLE password_resets ADD COLUMN IF NOT EXISTS used_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE password_resets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();

ALTER TABLE email_verifications ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();

COMMENT ON COLUMN password_resets.used_at IS 'When the reset token was consumed; NULL while pending.';

-- ============================================================
-- Verification query:
-- SELECT table_name, column_name FROM information_schema.columns
-- WHERE (table_name, column_name) IN (
--   ('password_resets','used_at'), ('password_resets','updated_at'),
--   ('email_verifications','updated_at'));
-- Expected: 3 rows
-- ============================================================
