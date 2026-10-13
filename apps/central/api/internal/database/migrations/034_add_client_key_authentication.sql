-- ============================================================
-- Migration 034: Clients that authenticate with a key
-- ============================================================
-- A confidential client proved who it is at the token endpoint with a secret,
-- and in no other way. A client that can keep a key may instead sign an
-- assertion with it (RFC 7523, `private_key_jwt`): the provider holds only
-- the public half, and nothing secret is shared or stored.
--
-- token_endpoint_auth_method: empty = what the kind of client implies (a
--   secret for a confidential client, none for a public one), or
--   'private_key_jwt'.
-- jwks:     the client's public keys, a JSON Web Key Set as text. Empty when
--           the keys are fetched from jwks_uri, or the client has none.
-- jwks_uri: where the authorization server fetches the client's keys.
--
-- All three are synced to the authorization server's client registration.
-- This migration sets them for no client.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS token_endpoint_auth_method VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS jwks TEXT NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS jwks_uri TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN clients.token_endpoint_auth_method IS 'How the client authenticates at the token endpoint when not implied by its kind: empty or private_key_jwt. Propagated to Hydra.';
COMMENT ON COLUMN clients.jwks IS 'Public keys (JSON Web Key Set) of a client that authenticates with a key. Propagated to Hydra client.jwks.';
COMMENT ON COLUMN clients.jwks_uri IS 'Where the keys of a client that authenticates with a key are fetched. Propagated to Hydra client.jwks_uri.';

-- Verification:
-- SELECT count(*) FROM clients WHERE token_endpoint_auth_method <> '';  -- expect 0
