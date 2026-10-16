-- ============================================================
-- Migration 044: Delegations users grant to clients
-- ============================================================
-- A user who consents to a client's `delegation` scope lets the client act
-- for them while they are not there, with the other scopes they consented
-- to, until they revoke it. The client receives a handle for the delegation
-- in the ID token of that consent and exchanges it, with its own credentials,
-- for a short-lived access token whose subject is the user (RFC 8693).
--
-- Only the handle's SHA-256 is kept (handle_hash), as for every other token.
-- A revoked delegation keeps its row (revoked_at) so the audit trail and the
-- token hook can tell "revoked" from "never existed"; it is no longer listed.
--
-- Additive: an API that does not know the table grants no delegation.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

CREATE TABLE IF NOT EXISTS delegation_grants (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id   UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    scopes      TEXT[] NOT NULL DEFAULT '{}',
    handle_hash VARCHAR(64) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    CONSTRAINT delegation_grants_handle_unique UNIQUE (handle_hash)
);

CREATE INDEX IF NOT EXISTS idx_delegation_grants_user_client
    ON delegation_grants (user_id, client_id, created_at DESC) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_delegation_grants_tenant
    ON delegation_grants (tenant_id, created_at DESC) WHERE revoked_at IS NULL;

COMMENT ON TABLE delegation_grants IS 'Delegations a user granted a client (consent to the delegation scope). Only the handle''s SHA-256 is stored; revoked rows stay with revoked_at set.';
