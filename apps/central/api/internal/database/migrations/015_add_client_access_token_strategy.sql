-- ============================================================
-- Migration 015: Per-Client Access Token Strategy
-- ============================================================
-- Adds an optional per-client access_token_strategy ('jwt' | 'opaque') that
-- maps to Hydra's client field of the same name, which overrides the global
-- `strategies.access_token` setting for that client only.
--
-- Why this exists: Authway ships Hydra's default opaque access tokens, so a
-- resource server cannot validate a token offline via standard OIDC discovery
-- (JWKS) — it would need Hydra admin introspection, and the admin API is
-- internal-only. docs/BACKEND_INTEGRATION.md nonetheless documented JWT
-- validation, so the documented contract was not deliverable.
--
-- Per-client opt-in (rather than flipping the global strategy) keeps the JWT
-- trade-off — offline validation means a token cannot be revoked before it
-- expires — scoped to the clients that explicitly ask for it, and leaves every
-- existing client's token format untouched.
--
-- NULL = inherit the deployment-wide strategy (currently opaque). This
-- migration deliberately enables nothing; which clients opt in is an
-- operational decision made per client after deploy.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee (empirically confirmed). Migration files must
-- contain no transaction-control statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS access_token_strategy VARCHAR(10) DEFAULT NULL;

ALTER TABLE clients DROP CONSTRAINT IF EXISTS clients_access_token_strategy_check;
ALTER TABLE clients ADD CONSTRAINT clients_access_token_strategy_check
    CHECK (access_token_strategy IS NULL OR access_token_strategy IN ('jwt', 'opaque'));

COMMENT ON COLUMN clients.access_token_strategy IS 'Per-client access token format: jwt | opaque. NULL inherits the deployment-wide strategies.access_token. Propagated to Hydra client.access_token_strategy.';

-- ============================================================
-- Migration Complete: Per-Client Access Token Strategy Added
-- ============================================================
--
-- Verification query:
-- SELECT column_name, data_type, column_default, is_nullable
-- FROM information_schema.columns
-- WHERE table_name = 'clients' AND column_name = 'access_token_strategy';
--
-- Expected: character varying, default NULL, is_nullable = YES.
--
-- No client is opted in by this migration:
-- SELECT count(*) FROM clients WHERE access_token_strategy IS NOT NULL;  -- expect 0
