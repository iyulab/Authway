-- ============================================================
-- Migration 028: Client back-channel logout address
-- ============================================================
-- An application that keeps its own server-side session had no way to learn
-- that the user signed out elsewhere: only the browser that signed out was
-- told. `backchannel_logout_uri` is where the authorization server POSTs a
-- logout token to the application's backend when a session of one of its
-- users ends (OpenID Connect Back-Channel Logout 1.0). It is synced to the
-- authorization server's client registration.
--
-- Empty = the client is not told, as before. This migration sets it for no
-- client.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS backchannel_logout_uri TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN clients.backchannel_logout_uri IS 'Where the client''s backend is told that a user''s session ended (logout token). Propagated to Hydra client.backchannel_logout_uri.';

-- Verification:
-- SELECT count(*) FROM clients WHERE backchannel_logout_uri <> '';  -- expect 0
