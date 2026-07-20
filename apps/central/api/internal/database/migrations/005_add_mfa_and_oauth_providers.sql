-- Migration: Add MFA/TOTP and additional OAuth provider fields
-- Version: 005
-- Date: 2025-12-07

-- No BEGIN;/COMMIT; here: RunMigrations wraps the whole run in one transaction,
-- and a nested COMMIT would commit that outer transaction early — dropping the
-- all-or-nothing guarantee for every migration that follows.

-- Add MFA/TOTP columns to users table
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS totp_secret TEXT,
    ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS totp_verified_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS recovery_codes TEXT;

-- Add additional OAuth provider columns
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS microsoft_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_id VARCHAR(255);

-- Create indexes for new OAuth provider IDs
CREATE INDEX IF NOT EXISTS idx_users_microsoft_id ON users(microsoft_id);
CREATE INDEX IF NOT EXISTS idx_users_apple_id ON users(apple_id);

-- Add Microsoft and Apple OAuth columns to clients table
ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS microsoft_oauth_enabled BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS microsoft_client_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS microsoft_client_secret TEXT,
    ADD COLUMN IF NOT EXISTS microsoft_tenant_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_oauth_enabled BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS apple_client_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_team_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_key_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_private_key TEXT;

COMMENT ON COLUMN users.totp_secret IS 'Encrypted TOTP secret for authenticator apps';
COMMENT ON COLUMN users.totp_enabled IS 'Whether MFA is enabled for this user';
COMMENT ON COLUMN users.recovery_codes IS 'JSON array of hashed recovery codes';


