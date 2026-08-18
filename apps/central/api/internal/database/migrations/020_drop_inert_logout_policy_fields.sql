-- ============================================================
-- Migration 020: Drop inert logout-policy fields
-- ============================================================
-- logout_redirect_policy, default_logout_uri, and allow_wildcard_logout
-- (added in migration 004) were stored and returned by the client API but
-- never read by any logout-validation path — post_logout_redirect_uris is
-- the only field Hydra's RP-initiated logout actually enforces against.
-- Dropped to stop the API from exposing settings that silently do nothing.
-- ============================================================

-- No BEGIN;/COMMIT; here: RunMigrations wraps the whole run in one transaction,
-- and a nested COMMIT would commit that outer transaction early — dropping the
-- all-or-nothing guarantee for every migration that follows.

DROP INDEX IF EXISTS idx_clients_logout_policy;

ALTER TABLE clients DROP COLUMN IF EXISTS logout_redirect_policy;
ALTER TABLE clients DROP COLUMN IF EXISTS default_logout_uri;
ALTER TABLE clients DROP COLUMN IF EXISTS allow_wildcard_logout;

-- ============================================================
-- Migration Complete: Inert Logout Policy Fields Dropped
-- ============================================================
--
-- Verification query:
-- SELECT column_name FROM information_schema.columns
-- WHERE table_name = 'clients'
-- AND column_name IN ('logout_redirect_policy', 'default_logout_uri', 'allow_wildcard_logout');
--
-- Expected result: 0 rows.
