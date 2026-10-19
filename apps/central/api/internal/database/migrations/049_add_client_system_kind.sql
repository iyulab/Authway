-- ============================================================
-- Migration 049: Which application of Authway a system client is
-- ============================================================
-- A tenant had at most one application Authway owns — the account screen's
-- client. The system tenant now has a second: the console's, which console
-- accounts sign in with. clients.system_kind says which one a system client
-- is ('account' or 'console'), and the one-per-tenant index becomes one per
-- tenant and kind. Existing system clients are the account screen's.
--
-- Additive: an API that does not know the column ignores it, and the index
-- still refuses a second client of the same kind. Rolling the API back leaves
-- a console client in the system tenant that the older API reads as the
-- account screen's, until the console client is deleted.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS system_kind TEXT;

UPDATE clients SET system_kind = 'account' WHERE is_system AND system_kind IS NULL;

COMMENT ON COLUMN clients.system_kind IS 'Which application of Authway a system client is: account (the account screen) or console (console accounts). Empty for clients that are not system clients.';

DROP INDEX IF EXISTS clients_one_system_per_tenant;

CREATE UNIQUE INDEX IF NOT EXISTS clients_one_system_kind_per_tenant
    ON clients (tenant_id, system_kind) WHERE is_system AND deleted_at IS NULL;
