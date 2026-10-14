-- ============================================================
-- Migration 036: Where an application starts a sign-in, and invitations for one
-- ============================================================
-- clients.initiate_login_uri is where the application starts a sign-in when a
-- third party asks it to (OpenID Connect Core 1.0 section 4); client_uri is
-- its home page. A user who arrives from outside a login flow — an emailed
-- invitation — is sent there.
--
-- invitations.client_id optionally names the application an invitation is
-- for, so the screen that accepts it can send the user on to it. A client
-- deleted later leaves the invitation without one.
--
-- Additive: an API that does not know the columns ignores them.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS initiate_login_uri TEXT NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS client_uri TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN clients.initiate_login_uri IS 'Where the application starts a sign-in (OpenID Connect Core 4). Empty = none registered.';
COMMENT ON COLUMN clients.client_uri IS 'The application''s home page. Empty = none registered.';

ALTER TABLE invitations ADD COLUMN IF NOT EXISTS client_id UUID REFERENCES clients(id) ON DELETE SET NULL;

COMMENT ON COLUMN invitations.client_id IS 'The application the invitation is for, if any (clients.id).';
