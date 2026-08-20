-- ============================================================
-- Migration 020: Hash Invitation Tokens at Rest
-- ============================================================
-- invitations.token (plaintext) -> token_hash (SHA-256 hex).
-- Completes the set started by 010 (admin sessions), 013 (password resets),
-- 014 (email verifications) and 019 (magic links): a database read must
-- never yield a usable token.
--
-- Unlike those four tables, invitations carries lasting audit value beyond
-- the live-token window (who was invited, who accepted, when) — an
-- accepted/declined/revoked/expired row is legitimate history, not a spent
-- nonce. So this migration does NOT delete existing rows the way 010/013/
-- 014/019 did. It only renames + retypes the column; the plaintext already
-- in it is re-hashed in place by invitation.BackfillTokenHashes, which runs
-- at application startup (see cmd/main.go), because SHA-256 is not
-- computable in plain SQL here — pgcrypto is not allow-listed on this
-- Azure Database for PostgreSQL instance (confirmed: CREATE EXTENSION
-- pgcrypto fails with "not allow-listed for users in Azure Database for
-- PostgreSQL"). The column stays plaintext-shaped (VARCHAR, no format
-- constraint) between migration and backfill, which is safe: nothing reads
-- it by content in that window, and the backfill runs unconditionally and
-- idempotently on every startup, so the window closes on the same deploy.
--
-- See issue: ISSUE-Authway-20260721-213000-invitation-token-plaintext-at-rest.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee (empirically confirmed). Migration files must
-- contain no transaction-control statements.
-- ============================================================

-- Drop the explicit index (migration 006) and the implicit UNIQUE constraint
-- it also created.
DROP INDEX IF EXISTS idx_invitations_token;
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_token_key;

-- Rename + retype: SHA-256 hex is always 64 chars. Existing plaintext values
-- (43-44 chars) fit within VARCHAR(64) unchanged; BackfillTokenHashes
-- converts them to their hash on next startup.
ALTER TABLE invitations RENAME COLUMN token TO token_hash;
ALTER TABLE invitations ALTER COLUMN token_hash TYPE VARCHAR(64);

-- Recreate a unique index on the (eventually all-hash) column.
CREATE UNIQUE INDEX idx_invitations_token_hash ON invitations(token_hash);

COMMENT ON COLUMN invitations.token_hash IS 'SHA-256 hex digest of the invitation token. Plaintext is never stored once invitation.BackfillTokenHashes has run.';

-- ============================================================
-- Verification query:
-- SELECT column_name, data_type, character_maximum_length
-- FROM information_schema.columns
-- WHERE table_name = 'invitations' AND column_name = 'token_hash';
-- Expected: token_hash, character varying, 64
-- ============================================================
