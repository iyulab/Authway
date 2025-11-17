-- Migration: Migrate schema_migrations table to new structure
-- Version: 001
-- Date: 2025-11-16
-- Description: Updates schema_migrations table from old structure (migration_file) to new structure (version)

-- Check if old structure exists and migrate
DO $$
DECLARE
    has_migration_file boolean;
    has_version boolean;
BEGIN
    -- Check if migration_file column exists (old structure)
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'schema_migrations'
        AND column_name = 'migration_file'
    ) INTO has_migration_file;

    -- Check if version column exists (new structure)
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'schema_migrations'
        AND column_name = 'version'
    ) INTO has_version;

    -- If old structure exists, migrate it
    IF has_migration_file AND NOT has_version THEN
        RAISE NOTICE 'Migrating schema_migrations table to new structure...';

        -- Create temporary backup of existing data
        CREATE TEMP TABLE schema_migrations_backup AS
        SELECT * FROM schema_migrations;

        -- Drop old table
        DROP TABLE schema_migrations;

        -- Create new structure
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

        -- Migrate old data to new structure
        -- Extract version from migration_file (e.g., "002_add_allowed_origins.sql" -> "002")
        INSERT INTO schema_migrations (version, name, executed_at, checksum, success)
        SELECT
            SUBSTRING(migration_file FROM '^(\d+)_'),  -- Extract version
            REGEXP_REPLACE(migration_file, '^(\d+)_(.+)\.sql$', '\2'),  -- Extract name
            applied_at,
            checksum,
            true  -- Old structure didn't track failures, assume success
        FROM schema_migrations_backup
        WHERE migration_file ~ '^\d+_';  -- Only migrate files with version prefix

        -- Add tracking table initialization record
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('000', 'init_migration_system', 0, true)
        ON CONFLICT (version) DO NOTHING;

        -- Record this migration itself
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('001', 'migrate_tracking_table', 0, true)
        ON CONFLICT (version) DO NOTHING;

        RAISE NOTICE 'Migration complete: % records migrated', (SELECT COUNT(*) FROM schema_migrations WHERE version != '000' AND version != '001');

    -- If version column already exists, just ensure tracking records exist
    ELSIF has_version THEN
        RAISE NOTICE 'schema_migrations table already using new structure';

        -- Ensure tracking table initialization record exists
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('000', 'init_migration_system', 0, true)
        ON CONFLICT (version) DO NOTHING;

        -- Record this migration
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('001', 'migrate_tracking_table', 0, true)
        ON CONFLICT (version) DO NOTHING;

    -- If neither exists, create new structure
    ELSE
        RAISE NOTICE 'Creating new schema_migrations table...';

        CREATE TABLE IF NOT EXISTS schema_migrations (
            id SERIAL PRIMARY KEY,
            version VARCHAR(255) NOT NULL UNIQUE,
            name VARCHAR(255) NOT NULL,
            executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
            execution_time_ms INTEGER,
            checksum VARCHAR(64),
            success BOOLEAN NOT NULL DEFAULT TRUE,
            error_message TEXT
        );

        CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
        CREATE INDEX IF NOT EXISTS idx_schema_migrations_executed_at ON schema_migrations(executed_at DESC);

        -- Record tracking table creation
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('000', 'init_migration_system', 0, true);

        -- Record this migration
        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('001', 'migrate_tracking_table', 0, true);
    END IF;
END $$;

-- Show migration status
SELECT
    version,
    name,
    executed_at,
    success
FROM schema_migrations
ORDER BY version;
