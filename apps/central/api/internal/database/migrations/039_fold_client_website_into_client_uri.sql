-- ============================================================
-- Migration 039: An application's home page is client_uri
-- ============================================================
-- clients.website and clients.client_uri (036) said the same thing: the
-- application's home page. client_uri is the one the contract has and the
-- screens use (where a user goes back to, where an invited user lands when
-- the application has no initiate_login_uri); website was shown by the
-- console and read by nothing else. The application stops reading and
-- writing website with this migration; it moves each value that is a valid
-- client_uri — an absolute https URL without a fragment — into client_uri
-- where client_uri is still empty.
--
-- Not moved: values that are not https, or rows that already have a
-- client_uri. Check what stays behind first if yours may hold some:
--   SELECT id, website, client_uri FROM clients
--    WHERE coalesce(website, '') <> '' AND deleted_at IS NULL;
--
-- Additive: the column stays, so the previous image can still be rolled back
-- to. It is dropped by a later migration, in a deployment of its own
-- (docs/DATABASE.md, "Changes that cannot be undone").
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

UPDATE clients
   SET client_uri = website
 WHERE client_uri = ''
   AND website ~ '^https://[^/?#[:space:]]+[^#[:space:]]*$'
   AND length(website) <= 2048;

COMMENT ON COLUMN clients.website IS 'Retired by migration 039 (moved into client_uri); no longer read or written. Dropped by a later migration.';
