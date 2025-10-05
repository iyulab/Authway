-- ========================================
-- Rollback Migration: 001_add_multi_tenancy
-- Version: 0.1.0
-- Date: 2025-10-05
-- Description: Rollback multi-tenancy changes
-- WARNING: This will delete all tenant-related data!
-- ========================================

BEGIN;

-- ========================================
-- 1. Drop New Tables
-- ========================================

DROP TABLE IF EXISTS password_resets CASCADE;
DROP TABLE IF EXISTS email_verifications CASCADE;
DROP TABLE IF EXISTS clients CASCADE;

-- ========================================
-- 2. Restore Users Table
-- ========================================

-- Remove tenant-related columns
ALTER TABLE users
    DROP COLUMN IF EXISTS tenant_id CASCADE,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS google_id,
    DROP COLUMN IF EXISTS github_id,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS last_login_at;

-- Restore original email unique constraint
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS unique_tenant_email;

ALTER TABLE users
    ADD CONSTRAINT users_email_key UNIQUE (email);

-- Drop tenant-related indexes
DROP INDEX IF EXISTS idx_users_tenant_id;
DROP INDEX IF EXISTS idx_users_google_id;
DROP INDEX IF EXISTS idx_users_github_id;

-- ========================================
-- 3. Restore Sessions Table
-- ========================================

ALTER TABLE sessions RENAME TO user_sessions;

ALTER TABLE user_sessions
    DROP COLUMN IF EXISTS tenant_id CASCADE,
    DROP COLUMN IF EXISTS ip_address,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS updated_at;

DROP INDEX IF EXISTS idx_sessions_tenant_id;
DROP INDEX IF EXISTS idx_sessions_expires_at;

-- ========================================
-- 4. Restore Consent Grants
-- ========================================

ALTER TABLE consent_grants
    DROP COLUMN IF EXISTS tenant_id CASCADE;

DROP INDEX IF EXISTS idx_consent_grants_tenant_id;

-- ========================================
-- 5. Recreate OAuth Clients Table
-- ========================================

CREATE TABLE oauth_clients (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id VARCHAR(255) UNIQUE NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    client_secret_hash VARCHAR(255),
    redirect_uris TEXT[],
    grant_types TEXT[] DEFAULT ARRAY['authorization_code', 'refresh_token'],
    response_types TEXT[] DEFAULT ARRAY['code'],
    scope TEXT DEFAULT 'openid profile email',
    owner_id UUID REFERENCES users(id) ON DELETE CASCADE,
    public BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_oauth_clients_client_id ON oauth_clients(client_id);
CREATE INDEX idx_oauth_clients_owner_id ON oauth_clients(owner_id);

-- ========================================
-- 6. Drop Tenants Table
-- ========================================

DROP TABLE IF EXISTS tenants CASCADE;

-- ========================================
-- 7. Drop Triggers and Functions
-- ========================================

DROP TRIGGER IF EXISTS update_tenants_updated_at ON tenants;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP TRIGGER IF EXISTS update_clients_updated_at ON clients;
DROP TRIGGER IF EXISTS update_sessions_updated_at ON sessions;

DROP FUNCTION IF EXISTS update_updated_at_column();

-- ========================================
-- Rollback Complete
-- ========================================

COMMIT;

-- Verification
-- SELECT COUNT(*) FROM users;
-- SELECT COUNT(*) FROM oauth_clients;
-- \d users
