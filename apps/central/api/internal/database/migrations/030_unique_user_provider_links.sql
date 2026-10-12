-- ============================================================
-- Migration 030: One account per sign-in provider account, per tenant
-- ============================================================
-- A link to a provider account (google_id, github_id, microsoft_id, apple_id)
-- identifies the account a person signs in to. The application looks the
-- link up before it links or creates an account, but two sign-ins arriving
-- together could both find nothing and link the same provider account to two
-- accounts. The database refuses the second now.
--
-- The rule mirrors the lookup: within a tenant, among accounts that are not
-- deleted. Accounts without a link (NULL) are not compared. The plain
-- single-column indexes these replace served the same lookups and are
-- dropped.
--
-- A database where two live accounts in one tenant already share a provider
-- account cannot take this migration; find them first:
--   SELECT tenant_id, google_id, count(*) FROM users
--   WHERE google_id IS NOT NULL AND deleted_at IS NULL
--   GROUP BY 1, 2 HAVING count(*) > 1;   -- and likewise for the other three
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_google_id
    ON users(tenant_id, google_id) WHERE google_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_github_id
    ON users(tenant_id, github_id) WHERE github_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_microsoft_id
    ON users(tenant_id, microsoft_id) WHERE microsoft_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_apple_id
    ON users(tenant_id, apple_id) WHERE apple_id IS NOT NULL AND deleted_at IS NULL;

DROP INDEX IF EXISTS idx_users_google_id;
DROP INDEX IF EXISTS idx_users_github_id;
DROP INDEX IF EXISTS idx_users_microsoft_id;
DROP INDEX IF EXISTS idx_users_apple_id;
