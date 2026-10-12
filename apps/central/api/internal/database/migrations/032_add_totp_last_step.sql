-- ============================================================
-- Migration 032: The time step of the last authenticator code accepted
-- ============================================================
-- An authenticator code stays good for its 30-second step and the one either
-- side of it, and nothing recorded that one had been used: a code seen once
-- could be presented again within that time. `totp_last_step` holds the step
-- of the last code accepted for the user; a code of that step or an earlier
-- one is refused (RFC 6238, section 5.2).
--
-- Additive: an API that does not know the column ignores it.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_step BIGINT;

COMMENT ON COLUMN users.totp_last_step IS 'Time step (Unix time / 30) of the last authenticator code accepted. NULL until one is.';
