-- Rollback Migration: Add Logout Redirect Policy Configuration
-- Version: 0.1.5
-- Date: 2025-01-14
-- Description: Rolls back logout redirect policy configuration

-- Drop index
DROP INDEX IF EXISTS idx_clients_logout_policy;

-- Remove check constraint
ALTER TABLE clients
DROP CONSTRAINT IF EXISTS check_logout_redirect_policy;

-- Remove columns
ALTER TABLE clients
DROP COLUMN IF EXISTS post_logout_redirect_uris,
DROP COLUMN IF EXISTS logout_redirect_policy,
DROP COLUMN IF EXISTS default_logout_uri,
DROP COLUMN IF EXISTS allow_wildcard_logout;

-- Rollback complete
