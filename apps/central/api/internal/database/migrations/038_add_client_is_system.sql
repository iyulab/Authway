-- ============================================================
-- Migration 038: Applications that are part of Authway
-- ============================================================
-- clients.is_system marks an application Authway itself owns: the account
-- screen's client, which a tenant gets the first time one of its users opens
-- that screen. The administration API shows it and refuses to change or delete
-- it; deleting the tenant takes it along.
--
-- One per tenant: the partial unique index makes two first requests that race
-- end with one row — the loser reads the winner's.
--
-- Additive: an API that does not know the column ignores it.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN clients.is_system IS 'Owned by Authway (the account screen''s client); not changed or deleted through the administration API.';

CREATE UNIQUE INDEX IF NOT EXISTS clients_one_system_per_tenant
    ON clients (tenant_id) WHERE is_system AND deleted_at IS NULL;
