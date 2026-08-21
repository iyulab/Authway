-- ============================================================
-- Migration 022: Service Clients (scoped M2M provisioning credentials)
-- ============================================================
-- Backs a tenant-scoped M2M provisioning credential: a service_client is a
-- Hydra client_credentials OAuth2Client (registered separately via the
-- Hydra admin API, not by this migration) mapped to exactly one Authway
-- tenant with an explicit scope allowlist. Hydra has no concept of an
-- Authway tenant, so this mapping lives here.
--
-- revoked_at follows the same soft-revoke convention as other credential
-- tables in this schema (e.g. invitations.status) — a non-NULL value means
-- the credential's requests must be rejected even if the underlying Hydra
-- client and any already-issued access token are still technically valid.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and
-- defeat its all-or-nothing guarantee. Migration files must contain no
-- transaction-control statements.
-- ============================================================

CREATE TABLE IF NOT EXISTS service_clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    hydra_client_id VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    granted_scopes TEXT[] NOT NULL DEFAULT '{}',
    revoked_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_service_clients_tenant ON service_clients(tenant_id);
CREATE INDEX IF NOT EXISTS idx_service_clients_hydra_client_id ON service_clients(hydra_client_id);

COMMENT ON TABLE service_clients IS 'Maps a Hydra client_credentials OAuth2Client to an Authway tenant with a scope allowlist, for tenant-scoped M2M client provisioning';
COMMENT ON COLUMN service_clients.hydra_client_id IS 'The client_id of the matching Hydra OAuth2Client (grant_types=[client_credentials]); Hydra itself is the credential store, this row is only the tenant + scope mapping';
COMMENT ON COLUMN service_clients.granted_scopes IS 'Scope allowlist this credential was provisioned with, e.g. admin.clients:write';
COMMENT ON COLUMN service_clients.revoked_at IS 'NULL = active. Non-NULL blocks every request even if the Hydra client / an already-issued token is still valid.';

-- ============================================================
-- Verification query:
-- SELECT column_name FROM information_schema.columns WHERE table_name = 'service_clients';
-- Expected: id, tenant_id, hydra_client_id, name, granted_scopes, revoked_at, created_at
-- ============================================================
