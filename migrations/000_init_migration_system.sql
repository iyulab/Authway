-- Migration tracking system initialization
-- This migration creates the tracking table used to manage all migrations

-- Create schema_migrations table if it doesn't exist
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

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
CREATE INDEX IF NOT EXISTS idx_schema_migrations_executed_at ON schema_migrations(executed_at DESC);

-- Insert record for this migration
INSERT INTO schema_migrations (version, name, execution_time_ms, success)
VALUES ('000', 'init_migration_system', 0, true)
ON CONFLICT (version) DO NOTHING;

COMMENT ON TABLE schema_migrations IS 'Tracks all database migrations applied to this database';
COMMENT ON COLUMN schema_migrations.version IS 'Migration version number (e.g., 001, 002, 003)';
COMMENT ON COLUMN schema_migrations.name IS 'Human-readable migration name';
COMMENT ON COLUMN schema_migrations.executed_at IS 'Timestamp when migration was executed';
COMMENT ON COLUMN schema_migrations.execution_time_ms IS 'Execution time in milliseconds';
COMMENT ON COLUMN schema_migrations.checksum IS 'SHA-256 checksum of migration file for integrity verification';
COMMENT ON COLUMN schema_migrations.success IS 'Whether migration completed successfully';
COMMENT ON COLUMN schema_migrations.error_message IS 'Error message if migration failed';
