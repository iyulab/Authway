-- ============================================================
-- Migration 052: Support access — an operator's named, timed entry into a
-- tenant
-- ============================================================
-- An operator's console account takes the platform's actions on every tenant
-- (creating, turning off, deleting, choosing who administers it) but reaches
-- into a tenant's contents — its users, applications, audit log and the rest —
-- only while it holds open support access to that tenant: one it started
-- with a reason, for an hour. Starting and ending it go into the tenant's
-- audit log; the row is what the API reads to admit the operator.
--
-- Open = ended_at IS NULL and expires_at in the future. A daily task closes
-- the expired ones (ended_by 'expired').
--
-- Additive: an API that does not know the table lets an operator into every
-- tenant as before.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

CREATE TABLE IF NOT EXISTS console_support_access (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    reason     VARCHAR(500) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    ended_at   TIMESTAMPTZ,
    ended_by   VARCHAR(16),
    CONSTRAINT console_support_access_ended CHECK (
        (ended_at IS NULL AND ended_by IS NULL)
        OR (ended_at IS NOT NULL AND ended_by IN ('operator', 'expired'))
    )
);

CREATE INDEX IF NOT EXISTS idx_console_support_access_open
    ON console_support_access (user_id, tenant_id) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_console_support_access_tenant
    ON console_support_access (tenant_id);

COMMENT ON TABLE console_support_access IS 'An operator''s support access to a tenant: started with a reason, open for an hour, recorded in the tenant''s audit log.';
