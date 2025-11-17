-- Migration: Add Logout Redirect Policy Configuration
-- Version: 003
-- Date: 2025-01-14
-- Description: Adds logout redirect policy configuration to OAuth clients

-- Check if migration already applied
DO $$
BEGIN
    -- Check if migration tracking table exists
    IF NOT EXISTS (
        SELECT FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename = 'schema_migrations'
    ) THEN
        -- Create migration tracking table if it doesn't exist
        CREATE TABLE schema_migrations (
            id SERIAL PRIMARY KEY,
            version VARCHAR(255) NOT NULL UNIQUE,
            name VARCHAR(255) NOT NULL,
            executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
            execution_time_ms INTEGER,
            checksum VARCHAR(64),
            success BOOLEAN NOT NULL DEFAULT TRUE,
            error_message TEXT
        );

        CREATE INDEX idx_schema_migrations_version ON schema_migrations(version);
        CREATE INDEX idx_schema_migrations_executed_at ON schema_migrations(executed_at DESC);

        -- Record the tracking table creation
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('000', 'init_migration_system', 0, true);
    END IF;

    -- Check if this migration already applied
    IF EXISTS (
        SELECT 1 FROM schema_migrations
        WHERE version = '003' AND success = true
    ) THEN
        RAISE NOTICE 'Migration 003 already applied, skipping...';
        RETURN;
    END IF;
END $$;

-- Begin migration transaction
BEGIN;

-- Record migration start
INSERT INTO schema_migrations (version, name, success)
VALUES ('003', 'add_logout_policy', false)
ON CONFLICT (version) DO UPDATE SET success = false, executed_at = CURRENT_TIMESTAMP;

-- Add logout policy columns to clients table (idempotent)
ALTER TABLE clients
ADD COLUMN IF NOT EXISTS post_logout_redirect_uris TEXT[] DEFAULT '{}',
ADD COLUMN IF NOT EXISTS logout_redirect_policy VARCHAR(20) DEFAULT 'strict',
ADD COLUMN IF NOT EXISTS default_logout_uri VARCHAR(512) NULL,
ADD COLUMN IF NOT EXISTS allow_wildcard_logout BOOLEAN DEFAULT false;

-- Add check constraint (idempotent with drop if exists)
DO $$
BEGIN
    -- Drop constraint if exists
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'check_logout_redirect_policy'
    ) THEN
        ALTER TABLE clients DROP CONSTRAINT check_logout_redirect_policy;
    END IF;

    -- Add constraint
    ALTER TABLE clients
    ADD CONSTRAINT check_logout_redirect_policy
    CHECK (logout_redirect_policy IN ('strict', 'lenient', 'disabled'));
END $$;

-- Create index on logout_redirect_policy (idempotent)
CREATE INDEX IF NOT EXISTS idx_clients_logout_policy
ON clients(logout_redirect_policy);

-- Migrate existing clients: Set default values (idempotent)
UPDATE clients
SET
  post_logout_redirect_uris = CASE
    WHEN post_logout_redirect_uris IS NULL AND redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0
    THEN redirect_uris
    WHEN post_logout_redirect_uris IS NULL
    THEN '{}'
    ELSE post_logout_redirect_uris
  END,
  default_logout_uri = CASE
    WHEN default_logout_uri IS NULL AND redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0
    THEN redirect_uris[1]
    ELSE default_logout_uri
  END,
  logout_redirect_policy = COALESCE(logout_redirect_policy, 'strict'),
  allow_wildcard_logout = COALESCE(allow_wildcard_logout, false);

-- Add comments for documentation
COMMENT ON COLUMN clients.post_logout_redirect_uris IS
'Whitelisted URIs for post-logout redirection (OIDC RP-Initiated Logout)';

COMMENT ON COLUMN clients.logout_redirect_policy IS
'Validation strictness: strict (required+validated), lenient (optional+validated), disabled (dev only)';

COMMENT ON COLUMN clients.default_logout_uri IS
'Default URI for lenient policy when post_logout_redirect_uri is not provided';

COMMENT ON COLUMN clients.allow_wildcard_logout IS
'Allow wildcard patterns in post_logout_redirect_uris (e.g., http://localhost:*, https://*.example.com)';

-- Update migration record to success
UPDATE schema_migrations
SET success = true,
    executed_at = CURRENT_TIMESTAMP
WHERE version = '003';

COMMIT;

-- Migration complete
SELECT
    version,
    name,
    executed_at,
    success
FROM schema_migrations
WHERE version = '003';
