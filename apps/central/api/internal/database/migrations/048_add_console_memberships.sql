-- ============================================================
-- Migration 048: Console accounts and what they may administer
-- ============================================================
-- People who administer Authway sign in with an account of one tenant, the
-- system tenant (tenants.is_system — at most one). A membership says what such
-- an account may administer:
--
--   role 'operator', tenant_id NULL — every tenant, as the admin API key does;
--   role 'owner' or 'admin', tenant_id — that one tenant.
--
-- One membership per account and tenant, and at most one operator membership
-- per account: two partial unique indexes, because a plain unique constraint
-- treats every NULL tenant_id as distinct.
--
-- Additive: an API that does not know the column and the table accepts no
-- console account and administers as before.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN tenants.is_system IS 'The tenant whose accounts sign in to the administration console. At most one.';

CREATE UNIQUE INDEX IF NOT EXISTS tenants_one_system
    ON tenants ((true)) WHERE is_system AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS console_memberships (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  UUID REFERENCES tenants(id) ON DELETE CASCADE,
    role       VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT console_memberships_role CHECK (
        (role = 'operator' AND tenant_id IS NULL)
        OR (role IN ('owner', 'admin') AND tenant_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS console_memberships_one_operator
    ON console_memberships (user_id) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS console_memberships_one_per_tenant
    ON console_memberships (user_id, tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_console_memberships_tenant
    ON console_memberships (tenant_id);

COMMENT ON TABLE console_memberships IS 'What a console account (a user of the system tenant) may administer: operator = every tenant, owner/admin = one tenant.';
