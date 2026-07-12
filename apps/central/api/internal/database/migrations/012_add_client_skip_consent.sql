-- ============================================================
-- Migration 012: Add Consent Skip Configuration to Clients
-- ============================================================
-- Adds per-client skip_consent / skip_logout_consent flags so first-party
-- trusted clients can bypass the OAuth consent and logout confirmation
-- screens. These map to Hydra's admin-API-only client fields of the same
-- name and are propagated on client create/update/secret-regeneration sync.
--
-- Root cause context: the columns were previously absent everywhere
-- (model -> sync struct -> Hydra), so consentReq.Skip was always false and
-- first-party clients saw the consent screen on every login.
-- See issue: consent-skip-not-plumbed.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee (empirically confirmed). Migration files must
-- contain no transaction-control statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS skip_consent BOOLEAN DEFAULT false;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS skip_logout_consent BOOLEAN DEFAULT false;

COMMENT ON COLUMN clients.skip_consent IS 'Bypass the OAuth consent screen for this client (first-party/trusted). Propagated to Hydra client.skip_consent.';
COMMENT ON COLUMN clients.skip_logout_consent IS 'Bypass the logout confirmation screen for this client. Propagated to Hydra client.skip_logout_consent.';

-- ============================================================
-- Migration Complete: Consent Skip Configuration Added
-- ============================================================
--
-- Verification query:
-- SELECT column_name, data_type, column_default
-- FROM information_schema.columns
-- WHERE table_name = 'clients'
-- AND column_name IN ('skip_consent', 'skip_logout_consent');
--
-- Expected results: both columns exist as boolean, default false.
