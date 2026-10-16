-- ============================================================
-- Migration 047: The algorithm a key-authenticating client signs with
-- ============================================================
-- A client that authenticates at the token endpoint with a signed assertion
-- (private_key_jwt) is checked by the authorization server with one
-- algorithm (RFC 7591 token_endpoint_auth_signing_alg). Until now none was
-- registered and the server took RS256, so a client registered with an EC
-- key could never authenticate.
--
-- Empty for every existing row: such a client keeps RS256, which is what it
-- was checked with.
--
-- Additive: an API that does not know the column registers clients as before.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS token_endpoint_auth_signing_alg VARCHAR(16) NOT NULL DEFAULT '';
