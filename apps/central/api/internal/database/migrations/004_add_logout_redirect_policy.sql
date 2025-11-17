-- ============================================================
-- Migration 004: Add Logout Redirect Policy and CORS Support
-- ============================================================
-- Adds logout redirect policy configuration and CORS allowed origins
-- to clients table for enhanced OAuth 2.0 logout flows
-- ============================================================

BEGIN;

-- ============================================================
-- Add CORS and Logout Redirect Policy columns to clients table
-- ============================================================

-- CORS Allowed Origins for dynamic CORS validation by reverse proxy
ALTER TABLE clients ADD COLUMN IF NOT EXISTS allowed_origins TEXT[] DEFAULT '{}';

-- Logout Redirect Policy Configuration
ALTER TABLE clients ADD COLUMN IF NOT EXISTS post_logout_redirect_uris TEXT[] DEFAULT '{}';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS logout_redirect_policy VARCHAR(20) DEFAULT 'strict';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS default_logout_uri TEXT;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS allow_wildcard_logout BOOLEAN DEFAULT false;

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_clients_logout_policy ON clients(logout_redirect_policy);

-- Add comments for documentation
COMMENT ON COLUMN clients.allowed_origins IS 'CORS allowed origins for browser-based OAuth flows (SPA clients)';
COMMENT ON COLUMN clients.post_logout_redirect_uris IS 'Allowed post-logout redirect URIs for OIDC logout flow';
COMMENT ON COLUMN clients.logout_redirect_policy IS 'Logout redirect validation policy: strict (exact match required), lenient (substring match), disabled (no validation)';
COMMENT ON COLUMN clients.default_logout_uri IS 'Default logout redirect URI when none provided by client';
COMMENT ON COLUMN clients.allow_wildcard_logout IS 'Allow wildcard patterns in post_logout_redirect_uri (e.g., https://*.example.com)';

COMMIT;

-- ============================================================
-- Migration Complete: Logout Redirect Policy Added
-- ============================================================
--
-- Verification queries:
-- SELECT column_name, data_type, column_default
-- FROM information_schema.columns
-- WHERE table_name = 'clients'
-- AND column_name IN ('allowed_origins', 'post_logout_redirect_uris', 'logout_redirect_policy', 'default_logout_uri', 'allow_wildcard_logout');
--
-- Expected results: All 5 columns should exist with proper types and defaults
