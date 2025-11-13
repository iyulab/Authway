-- Rollback Migration: Remove allowed_origins from clients table
-- Date: 2025-11-13

-- Drop index
DROP INDEX IF EXISTS idx_clients_allowed_origins;

-- Remove allowed_origins column
ALTER TABLE clients
DROP COLUMN IF EXISTS allowed_origins;

-- Verify rollback
-- SELECT column_name FROM information_schema.columns WHERE table_name = 'clients' AND column_name = 'allowed_origins';
-- Should return no rows
