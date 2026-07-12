-- ============================================================
-- Migration 013: Hash Password Reset Tokens at Rest
-- ============================================================
-- password_resets.token (plaintext) -> token_hash (SHA-256 hex).
-- A database read must never yield a usable reset token. Mirrors migration
-- 010 (admin_sessions token hashing).
--
-- All existing pending resets are invalidated: plaintext cannot be
-- reverse-hashed, so any outstanding reset link stops working and the user
-- must request a new one. Reset tokens are short-lived (1h), so the impact is
-- limited to in-flight requests.
--
-- See issue: sensitive-material-plaintext.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee (empirically confirmed). Migration files must
-- contain no transaction-control statements.
-- ============================================================

-- Invalidate all existing (plaintext) reset tokens.
DELETE FROM password_resets;

-- Drop the two token indexes/constraints created in migration 000
-- (explicit index + implicit UNIQUE constraint).
DROP INDEX IF EXISTS idx_password_resets_token;
ALTER TABLE password_resets DROP CONSTRAINT IF EXISTS password_resets_token_key;

-- Rename + retype: SHA-256 hex is always 64 chars.
ALTER TABLE password_resets RENAME COLUMN token TO token_hash;
ALTER TABLE password_resets ALTER COLUMN token_hash TYPE VARCHAR(64);

-- Recreate a unique index on the hash.
CREATE UNIQUE INDEX idx_password_resets_token_hash ON password_resets(token_hash);

COMMENT ON COLUMN password_resets.token_hash IS 'SHA-256 hex digest of the reset token. Plaintext is never stored.';

-- ============================================================
-- Verification query:
-- SELECT column_name, data_type, character_maximum_length
-- FROM information_schema.columns
-- WHERE table_name = 'password_resets' AND column_name = 'token_hash';
-- Expected: token_hash, character varying, 64
-- ============================================================
