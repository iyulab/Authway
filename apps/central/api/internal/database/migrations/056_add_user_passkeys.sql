-- ============================================================
-- Migration 056: Passkeys — one row per WebAuthn credential an account
-- signs in with
-- ============================================================
-- A passkey is a key pair the user's device keeps; the account keeps the public
-- half, the credential id the device names it by, and the counters and flags the
-- device reported when it was last used. An account may have any number of them.
--
-- credential_id is unique within a tenant, not across them: a credential id is
-- chosen by whoever creates the credential, so a global rule would let anyone who
-- knows an id (or guesses one) keep it from being registered anywhere else.
-- Every lookup by credential id is made inside the tenant of the sign-in.
--
-- Additive only: nothing reads this table until the passkey endpoints ship, and
-- an API that does not know it keeps working.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

CREATE TABLE IF NOT EXISTS user_passkeys (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL,
    public_key    BYTEA NOT NULL,
    -- The signature counter the device reported at its last use (0 for a device
    -- that keeps none, which is every synced passkey).
    sign_count    BIGINT NOT NULL DEFAULT 0,
    aaguid        BYTEA NOT NULL DEFAULT ''::bytea,
    -- Comma-separated authenticator transports the device offered at
    -- registration ("internal,hybrid"). Kept with the passkey; a sign-in names
    -- no credential, so it is not sent back to the browser.
    transports    VARCHAR(255) NOT NULL DEFAULT '',
    -- The authenticator-data flags byte of the last ceremony: user present,
    -- user verified, backup eligible, backup state. Backup eligibility must not
    -- change, and the library checks it against this.
    flags         SMALLINT NOT NULL DEFAULT 0,
    -- What the user calls it ("Work laptop"); the device's own name for it is
    -- not available to a web page.
    name          VARCHAR(100) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_passkeys_tenant_credential
    ON user_passkeys (tenant_id, credential_id);
CREATE INDEX IF NOT EXISTS idx_user_passkeys_user ON user_passkeys (user_id);
