-- ============================================================
-- Migration 019: Hash Magic Link Tokens at Rest
-- ============================================================
-- magic_link_tokens.token (plaintext) -> token_hash (SHA-256 hex).
-- Completes the set started by 010 (admin sessions), 013 (password resets) and
-- 014 (email verifications): a database read must never yield a usable token.
--
-- This table was missed by that pass, and it is the one that matters most —
-- a magic link is a login factor on its own, so read access to this column was
-- read access to any pending account.
--
-- All existing links are invalidated (plaintext cannot be reverse-hashed).
-- They live 15 minutes, so the blast radius is one retry.
--
-- See issue: ISSUE-Authway-20260721-160500-magic-link-token-plaintext-at-rest.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee (empirically confirmed). Migration files must
-- contain no transaction-control statements.
-- ============================================================

-- Invalidate all existing (plaintext) magic links.
DELETE FROM magic_link_tokens;

-- Drop the token index from migration 006 and any unique constraint GORM or a
-- later migration may have added.
DROP INDEX IF EXISTS idx_magic_link_token;
ALTER TABLE magic_link_tokens DROP CONSTRAINT IF EXISTS magic_link_tokens_token_key;

-- Rename + retype: SHA-256 hex is always 64 chars.
ALTER TABLE magic_link_tokens RENAME COLUMN token TO token_hash;
ALTER TABLE magic_link_tokens ALTER COLUMN token_hash TYPE VARCHAR(64);

-- Recreate a unique index on the hash.
CREATE UNIQUE INDEX idx_magic_link_tokens_token_hash ON magic_link_tokens(token_hash);

COMMENT ON COLUMN magic_link_tokens.token_hash IS 'SHA-256 hex digest of the magic link token. Plaintext is never stored.';

-- ============================================================
-- Verification query:
-- SELECT column_name, data_type, character_maximum_length
-- FROM information_schema.columns
-- WHERE table_name = 'magic_link_tokens' AND column_name = 'token_hash';
-- Expected: token_hash, character varying, 64
-- ============================================================
