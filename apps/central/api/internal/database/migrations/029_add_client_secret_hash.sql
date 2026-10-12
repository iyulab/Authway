-- ============================================================
-- Migration 029: Client secret hash
-- ============================================================
-- A confidential client's secret was stored as it is. Nothing needs it back:
-- the authorization server authenticates the client with its own copy, and an
-- update that names no secret keeps the one it has. Anyone able to read this
-- table could read every client's secret.
--
-- `client_secret_hash` holds a password hash of the secret. New and
-- regenerated secrets are stored only as the hash; `client_secret` is then
-- written empty. On start the API hashes the secrets of existing rows into
-- this column (it does not remove them — see below).
--
-- Removing the secrets already stored is deliberately NOT part of this
-- migration: it cannot be undone. Once every row has a hash, run:
--
--   UPDATE clients SET client_secret = '' WHERE client_secret_hash <> '';
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS client_secret_hash TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN clients.client_secret_hash IS 'Password hash of the client secret. The secret itself is not stored.';
COMMENT ON COLUMN clients.client_secret IS 'Legacy: the secret itself, for rows created before client_secret_hash. Written empty since.';

-- Verification (after the API has started once):
-- SELECT count(*) FROM clients WHERE client_secret <> '' AND client_secret_hash = '';  -- expect 0
