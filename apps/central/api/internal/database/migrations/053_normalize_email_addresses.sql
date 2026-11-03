-- ============================================================
-- Migration 053: An address is one address whatever the case of its letters
-- ============================================================
-- People type the same address in different cases and identity providers
-- return it in lower case, but accounts and invitations were found by an
-- exact match — so "Alice@Corp.com" could not sign in as "alice@corp.com",
-- and a social sign-in could create a second account beside it.
--
-- The application now stores addresses with ASCII capitals in lower case and
-- compares them as stored. This migration brings the stored rows to that form
-- and has the database refuse an account address that is not in it, so the
-- existing unique index on (tenant_id, email) refuses a second account for an
-- address that differs only in case.
--
-- Only the ASCII letters A-Z are folded (translate, not lower()): lower() folds
-- by the database's locale and would also make letters outside ASCII — the
-- Kelvin sign for "k" — equal to ASCII ones, and the application does not.
--
-- A database where two accounts in one tenant already share an address that
-- differs only in the case of its ASCII letters cannot take this migration;
-- find them first:
--   SELECT tenant_id,
--          translate(email, 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz'),
--          count(*)
--   FROM users GROUP BY 1, 2 HAVING count(*) > 1;
--
-- An API that is still running while this applies would fail to create an
-- account whose address has capitals; the new one does not make such rows.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

UPDATE users
SET email = translate(btrim(email), 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz')
WHERE email <> translate(btrim(email), 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz');

UPDATE invitations
SET email = translate(btrim(email), 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz')
WHERE email <> translate(btrim(email), 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz');

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_ascii_lower;
ALTER TABLE users ADD CONSTRAINT users_email_ascii_lower
    CHECK (email = translate(email, 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz'));
