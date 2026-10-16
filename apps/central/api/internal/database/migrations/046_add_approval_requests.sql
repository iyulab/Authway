-- ============================================================
-- Migration 046: Approvals a client asks a user for while they are away
-- ============================================================
-- A client acting for a user (a delegation, migration 044) asks the user to
-- approve one action. The user approves or denies it on Authway's approval
-- screen, signed in; the client polls the token endpoint and, once approved,
-- receives one short-lived access token bound to the approval.
--
-- Only the SHA-256 of the polling secret (auth_req_id) is kept.
-- authorization_details is the client's own description of the action
-- (RFC 9396 shape), kept as given and put in the token as given.
--
-- Additive: an API that does not know the table accepts no approval request.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

CREATE TABLE IF NOT EXISTS approval_requests (
    id                    UUID PRIMARY KEY,
    tenant_id             UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id             UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    delegation_id         UUID NOT NULL REFERENCES delegation_grants(id) ON DELETE CASCADE,
    auth_req_hash         VARCHAR(64) NOT NULL,
    binding_message       VARCHAR(200) NOT NULL DEFAULT '',
    scopes                TEXT[] NOT NULL DEFAULT '{}',
    authorization_details JSONB,
    status                VARCHAR(16) NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    decided_at            TIMESTAMPTZ,
    consumed_at           TIMESTAMPTZ,
    last_polled_at        TIMESTAMPTZ,
    CONSTRAINT approval_requests_auth_req_unique UNIQUE (auth_req_hash),
    CONSTRAINT approval_requests_status CHECK (status IN ('pending', 'approved', 'denied'))
);

CREATE INDEX IF NOT EXISTS idx_approval_requests_user ON approval_requests (user_id, created_at DESC);

COMMENT ON TABLE approval_requests IS 'Approvals a client asked a user for while they were away. Expired is expires_at past; consumed is consumed_at set (one token per approval). Only the polling secret''s SHA-256 is stored.';
