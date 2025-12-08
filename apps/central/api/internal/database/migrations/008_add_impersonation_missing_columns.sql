-- Migration: Add missing columns to impersonation_sessions table
-- Version: 008
-- Date: 2025-12-08
-- Purpose: Add tenant_id, admin_email, target_user_email, expires_at columns

BEGIN;

-- ============================================================
-- 1. Add tenant_id column
-- ============================================================

-- Add tenant_id column (nullable first for existing data)
ALTER TABLE impersonation_sessions
ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Update existing rows to use default tenant if they exist
UPDATE impersonation_sessions
SET tenant_id = (
    SELECT t.id FROM tenants t
    WHERE t.slug = 'default'
    LIMIT 1
)
WHERE tenant_id IS NULL;

-- If no default tenant, try to get tenant from admin user
UPDATE impersonation_sessions s
SET tenant_id = (
    SELECT u.tenant_id FROM users u WHERE u.id = s.admin_id LIMIT 1
)
WHERE s.tenant_id IS NULL;

-- Set NOT NULL constraint after populating existing rows
-- Only if there's data with NULL tenant_id, delete it (dev only)
DELETE FROM impersonation_sessions WHERE tenant_id IS NULL;

-- Now add NOT NULL constraint
ALTER TABLE impersonation_sessions
ALTER COLUMN tenant_id SET NOT NULL;

-- Add foreign key constraint
ALTER TABLE impersonation_sessions
ADD CONSTRAINT fk_impersonation_tenant
FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

-- Create index on tenant_id
CREATE INDEX IF NOT EXISTS idx_impersonation_tenant ON impersonation_sessions(tenant_id);

-- ============================================================
-- 2. Add admin_email column
-- ============================================================

ALTER TABLE impersonation_sessions
ADD COLUMN IF NOT EXISTS admin_email VARCHAR(255);

-- Populate from users table
UPDATE impersonation_sessions s
SET admin_email = (
    SELECT u.email FROM users u WHERE u.id = s.admin_id LIMIT 1
)
WHERE s.admin_email IS NULL;

-- Set NOT NULL after populating
ALTER TABLE impersonation_sessions
ALTER COLUMN admin_email SET NOT NULL;

-- ============================================================
-- 3. Add target_user_email column
-- ============================================================

ALTER TABLE impersonation_sessions
ADD COLUMN IF NOT EXISTS target_user_email VARCHAR(255);

-- Populate from users table
UPDATE impersonation_sessions s
SET target_user_email = (
    SELECT u.email FROM users u WHERE u.id = s.target_user_id LIMIT 1
)
WHERE s.target_user_email IS NULL;

-- Set NOT NULL after populating
ALTER TABLE impersonation_sessions
ALTER COLUMN target_user_email SET NOT NULL;

-- ============================================================
-- 4. Add expires_at column
-- ============================================================

ALTER TABLE impersonation_sessions
ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;

-- Set default expiry for existing rows (1 hour from started_at)
UPDATE impersonation_sessions
SET expires_at = started_at + INTERVAL '1 hour'
WHERE expires_at IS NULL;

-- Set NOT NULL constraint
ALTER TABLE impersonation_sessions
ALTER COLUMN expires_at SET NOT NULL;

-- Create index on expires_at for cleanup queries
CREATE INDEX IF NOT EXISTS idx_impersonation_expires ON impersonation_sessions(expires_at);

COMMIT;

-- ============================================================
-- Migration Complete
-- ============================================================
--
-- Added columns:
-- - tenant_id: UUID NOT NULL with FK to tenants and index
-- - admin_email: VARCHAR(255) NOT NULL (denormalized for display)
-- - target_user_email: VARCHAR(255) NOT NULL (denormalized for display)
-- - expires_at: TIMESTAMP NOT NULL with index
--
-- Verification:
-- SELECT column_name, data_type, is_nullable
-- FROM information_schema.columns
-- WHERE table_name = 'impersonation_sessions';
--
