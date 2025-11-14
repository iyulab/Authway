-- Migration: Add Logout Redirect Policy Configuration
-- Version: 0.1.5
-- Date: 2025-01-14
-- Description: Adds logout redirect policy configuration to OAuth clients

-- Add logout policy columns to clients table
ALTER TABLE clients
ADD COLUMN IF NOT EXISTS post_logout_redirect_uris TEXT[] DEFAULT '{}',
ADD COLUMN IF NOT EXISTS logout_redirect_policy VARCHAR(20) DEFAULT 'strict',
ADD COLUMN IF NOT EXISTS default_logout_uri VARCHAR(512) NULL,
ADD COLUMN IF NOT EXISTS allow_wildcard_logout BOOLEAN DEFAULT false;

-- Add check constraint for logout_redirect_policy
ALTER TABLE clients
ADD CONSTRAINT check_logout_redirect_policy
CHECK (logout_redirect_policy IN ('strict', 'lenient', 'disabled'));

-- Create index on logout_redirect_policy for query performance
CREATE INDEX IF NOT EXISTS idx_clients_logout_policy
ON clients(logout_redirect_policy);

-- Migrate existing clients: Set default values
-- Strategy: Use first redirect_uri as default_logout_uri if available
UPDATE clients
SET
  post_logout_redirect_uris = CASE
    WHEN redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0
    THEN redirect_uris
    ELSE '{}'
  END,
  default_logout_uri = CASE
    WHEN redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0
    THEN redirect_uris[1]
    ELSE NULL
  END,
  logout_redirect_policy = 'strict',
  allow_wildcard_logout = false
WHERE post_logout_redirect_uris IS NULL;

-- Add comments for documentation
COMMENT ON COLUMN clients.post_logout_redirect_uris IS
'Whitelisted URIs for post-logout redirection (OIDC RP-Initiated Logout)';

COMMENT ON COLUMN clients.logout_redirect_policy IS
'Validation strictness: strict (required+validated), lenient (optional+validated), disabled (dev only)';

COMMENT ON COLUMN clients.default_logout_uri IS
'Default URI for lenient policy when post_logout_redirect_uri is not provided';

COMMENT ON COLUMN clients.allow_wildcard_logout IS
'Allow wildcard patterns in post_logout_redirect_uris (e.g., http://localhost:*, https://*.example.com)';

-- Migration complete
-- Next steps:
-- 1. Deploy application code that uses these fields
-- 2. Update Hydra clients with post_logout_redirect_uris
-- 3. Update documentation
