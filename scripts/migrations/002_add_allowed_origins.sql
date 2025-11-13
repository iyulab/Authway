-- Migration: Add allowed_origins to clients table for dynamic CORS management
-- Purpose: Enable OAuth 2.0 compliant dynamic CORS validation via reverse proxy
-- Date: 2025-11-13
-- Related Issue: CORS token endpoint accessibility for multi-tenant SPA clients

-- Add allowed_origins column to clients table
ALTER TABLE clients
ADD COLUMN IF NOT EXISTS allowed_origins TEXT[] DEFAULT '{}';

-- Add comment to explain the purpose
COMMENT ON COLUMN clients.allowed_origins IS 'List of allowed origins for CORS validation. Used by reverse proxy to dynamically add CORS headers for /oauth2/token endpoint.';

-- Update existing All.Manual client with production origins (example)
-- UPDATE clients
-- SET allowed_origins = ARRAY[
--   'https://manuals.alldot.ai',
--   'https://nice-moss-08ac84200.3.azurestaticapps.net'
-- ]
-- WHERE client_id = 'authway_2qfEM6ccGYfmxh8bC6hjng';

-- Example: Add localhost origins for development clients
-- UPDATE clients
-- SET allowed_origins = ARRAY[
--   'http://localhost:3000',
--   'http://localhost:5173',
--   'http://localhost:9000'
-- ]
-- WHERE client_id LIKE 'dev_%';

-- Create index for faster origin lookups by reverse proxy
CREATE INDEX IF NOT EXISTS idx_clients_allowed_origins
ON clients USING GIN (allowed_origins);

-- Verify migration
-- SELECT client_id, name, allowed_origins FROM clients;
