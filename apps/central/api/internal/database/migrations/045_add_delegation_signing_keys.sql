-- ============================================================
-- Migration 045: The key Authway signs delegation assertions with
-- ============================================================
-- When a client exchanges a delegation, Authway asks the authorization server
-- for the user's token with an assertion (RFC 7523) it signs itself; the
-- authorization server trusts this key for that. The assertion never leaves
-- the deployment: the authorization server stays the only issuer of tokens.
--
-- The private key is sealed with the at-rest key (AUTHWAY_TOTP_ENCRYPTION_KEY),
-- like the other secrets the API must read back. The newest row is the key in
-- use; every replica reads the same one.
--
-- Additive: an API that does not know the table offers no token exchange.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

CREATE TABLE IF NOT EXISTS delegation_signing_keys (
    kid         VARCHAR(64) PRIMARY KEY,
    private_key TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE delegation_signing_keys IS 'Keys Authway signs delegation assertions with (P-256, PKCS#8, sealed at rest). The newest is in use.';
