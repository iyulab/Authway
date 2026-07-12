-- ============================================================
-- Migration 014: Hash Email Verification Tokens at Rest
-- ============================================================
-- email_verifications.token (plaintext) -> token_hash (SHA-256 hex).
-- Mirrors migration 013 (password reset tokens) / 010 (admin sessions):
-- a database read must never yield a usable verification token.
--
-- All existing pending verifications are invalidated (plaintext cannot be
-- reverse-hashed); affected users simply request a new verification email.
--
-- See issue: sensitive-material-plaintext.
-- ============================================================

BEGIN;

-- Invalidate all existing (plaintext) verification tokens.
DELETE FROM email_verifications;

-- Drop the two token indexes/constraints created in migration 000
-- (explicit index + implicit UNIQUE constraint).
DROP INDEX IF EXISTS idx_email_verifications_token;
ALTER TABLE email_verifications DROP CONSTRAINT IF EXISTS email_verifications_token_key;

-- Rename + retype: SHA-256 hex is always 64 chars.
ALTER TABLE email_verifications RENAME COLUMN token TO token_hash;
ALTER TABLE email_verifications ALTER COLUMN token_hash TYPE VARCHAR(64);

-- Recreate a unique index on the hash.
CREATE UNIQUE INDEX idx_email_verifications_token_hash ON email_verifications(token_hash);

COMMENT ON COLUMN email_verifications.token_hash IS 'SHA-256 hex digest of the verification token. Plaintext is never stored.';

COMMIT;

-- ============================================================
-- Verification query:
-- SELECT column_name, data_type, character_maximum_length
-- FROM information_schema.columns
-- WHERE table_name = 'email_verifications' AND column_name = 'token_hash';
-- Expected: token_hash, character varying, 64
-- ============================================================
