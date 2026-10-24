-- ============================================================
-- Migration 051: Drop clients.client_secret and allow_email_login; remove
-- the accounts that were only marked deleted, and users.deleted_at with them
-- ============================================================
-- clients.client_secret held a client's secret before secrets were hashed;
-- allow_email_login was a second switch for password sign-in. The previous
-- deployment stopped reading and writing both (migration 050).
--
-- Deleting an account used to set users.deleted_at and leave the row — the
-- address, the password hash, the provider ids — and every row keyed by it,
-- since the cascades never ran. The previous deployment deletes accounts
-- outright; this removes the ones deleted before it (their claims, terms
-- acceptances, delegations, approvals and memberships go by cascade;
-- invitations keep their row with the account cleared), then the column and
-- the uniqueness rules that were conditional on it.
--
-- What it deletes — query the target first:
--   SELECT count(*) FROM clients WHERE client_secret <> '';   -- expected 0
--   SELECT count(*) FROM users WHERE deleted_at IS NOT NULL;
--
-- Destructive, so it ships in a deployment of its own, after the one that
-- stopped using the columns and started deleting accounts outright.
--
-- There are no down migrations; the deleted accounts and the dropped values
-- cannot come back. To have the columns back as they were defined:
--   ALTER TABLE clients ADD COLUMN client_secret TEXT NOT NULL DEFAULT '';
--   ALTER TABLE clients ADD COLUMN allow_email_login BOOLEAN DEFAULT true;
--   ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE clients DROP COLUMN IF EXISTS client_secret;
ALTER TABLE clients DROP COLUMN IF EXISTS allow_email_login;

DELETE FROM users WHERE deleted_at IS NOT NULL;

-- Dropping the column drops the indexes whose condition names it.
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_tenant_email ON users (tenant_id, email);
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_google_id ON users (tenant_id, google_id) WHERE google_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_github_id ON users (tenant_id, github_id) WHERE github_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_microsoft_id ON users (tenant_id, microsoft_id) WHERE microsoft_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_apple_id ON users (tenant_id, apple_id) WHERE apple_id IS NOT NULL;
