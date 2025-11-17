-- Migration: Add allowed_origins to clients table for dynamic CORS management
-- Version: 002
-- Purpose: Enable OAuth 2.0 compliant dynamic CORS validation via reverse proxy
-- Date: 2025-11-13
-- Related Issue: CORS token endpoint accessibility for multi-tenant SPA clients

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
        WHERE version = '002' AND success = true
    ) THEN
        RAISE NOTICE 'Migration 002 already applied, skipping...';
        RETURN;
    END IF;
END $$;

-- Begin migration transaction
BEGIN;

-- Record migration start
INSERT INTO schema_migrations (version, name, success)
VALUES ('002', 'add_allowed_origins', false)
ON CONFLICT (version) DO UPDATE SET success = false, executed_at = CURRENT_TIMESTAMP;

-- Add allowed_origins column to clients table (idempotent)
ALTER TABLE clients
ADD COLUMN IF NOT EXISTS allowed_origins TEXT[] DEFAULT '{}';

-- Add comment to explain the purpose
COMMENT ON COLUMN clients.allowed_origins IS 'List of allowed origins for CORS validation. Used by reverse proxy to dynamically add CORS headers for /oauth2/token endpoint.';

-- Create index for faster origin lookups by reverse proxy (idempotent)
CREATE INDEX IF NOT EXISTS idx_clients_allowed_origins
ON clients USING GIN (allowed_origins);

-- Update migration record to success
UPDATE schema_migrations
SET success = true,
    executed_at = CURRENT_TIMESTAMP
WHERE version = '002';

COMMIT;

-- Migration complete
SELECT
    version,
    name,
    executed_at,
    success
FROM schema_migrations
WHERE version = '002';
