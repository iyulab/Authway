-- ========================================
-- Migration: Add Multi-Tenancy Support
-- Version: 0.1.0
-- Date: 2025-10-05
-- Description: Refactor to support multi-tenant architecture
--              - Add tenants table
--              - Add tenant_id to users and clients
--              - Remove isolation_mode (Tenant = isolation unit)
-- ========================================

BEGIN;

-- ========================================
-- 1. Create Tenants Table
-- ========================================

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Basic Information
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,

    -- Settings (JSONB for flexibility)
    settings JSONB DEFAULT '{
        "require_email_verification": true,
        "password_min_length": 8,
        "session_timeout": 60,
        "allowed_domains": []
    }'::jsonb,

    -- Branding
    logo TEXT,
    primary_color VARCHAR(20) DEFAULT '#4F46E5',

    -- Status
    active BOOLEAN DEFAULT true,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for Tenants
CREATE INDEX idx_tenants_slug ON tenants(slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_tenants_active ON tenants(active) WHERE deleted_at IS NULL;

COMMENT ON TABLE tenants IS 'Tenant isolation unit - each tenant has independent users and apps';
COMMENT ON COLUMN tenants.slug IS 'URL-friendly identifier for tenant';
COMMENT ON COLUMN tenants.settings IS 'Tenant-specific settings (email verification, password policy, etc.)';

-- ========================================
-- 2. Create Default Tenant
-- ========================================

INSERT INTO tenants (id, name, slug, description, active)
VALUES (
    '00000000-0000-0000-0000-000000000001'::uuid,
    'Default',
    'default',
    'Default tenant for backward compatibility and initial setup',
    true
)
ON CONFLICT (slug) DO NOTHING;

-- ========================================
-- 3. Modify Users Table
-- ========================================

-- Add tenant_id column
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Migrate existing users to default tenant
UPDATE users
SET tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL after migration
ALTER TABLE users
    ALTER COLUMN tenant_id SET NOT NULL;

-- Drop old unique constraint on email (email is only unique within tenant)
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_email_key;

-- Add composite unique constraint: (tenant_id, email)
-- Same email can exist in different tenants
ALTER TABLE users
    ADD CONSTRAINT unique_tenant_email UNIQUE (tenant_id, email);

-- Add additional columns for OAuth providers
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS provider VARCHAR(50) DEFAULT 'local',
    ADD COLUMN IF NOT EXISTS google_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS github_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS active BOOLEAN DEFAULT true,
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE;

-- Update indexes
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_google_id ON users(google_id) WHERE google_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_github_id ON users(github_id) WHERE github_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NULL;

COMMENT ON COLUMN users.tenant_id IS 'Tenant isolation - users belong to one tenant';
COMMENT ON CONSTRAINT unique_tenant_email ON users IS 'Email unique per tenant - same email can exist in different tenants';

-- ========================================
-- 4. Migrate OAuth Clients to Clients Table
-- ========================================

-- Drop old oauth_clients table if exists and recreate as clients
DROP TABLE IF EXISTS oauth_clients CASCADE;

CREATE TABLE IF NOT EXISTS clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Tenant Relationship
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- OAuth 2.0 Basic Information
    client_id VARCHAR(255) UNIQUE NOT NULL,
    client_secret VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Metadata
    website TEXT,
    logo TEXT,

    -- OAuth 2.0 Configuration
    redirect_uris TEXT[] NOT NULL,
    grant_types TEXT[] NOT NULL DEFAULT ARRAY['authorization_code', 'refresh_token'],
    scopes TEXT[] NOT NULL DEFAULT ARRAY['openid', 'profile', 'email'],

    -- Client Type
    public BOOLEAN DEFAULT false,  -- true for mobile/SPA clients

    -- Client-specific Google OAuth (optional)
    google_oauth_enabled BOOLEAN DEFAULT false,
    google_client_id VARCHAR(255),
    google_client_secret VARCHAR(255),
    google_redirect_uri TEXT,

    -- Client-specific GitHub OAuth (optional)
    github_oauth_enabled BOOLEAN DEFAULT false,
    github_client_id VARCHAR(255),
    github_client_secret VARCHAR(255),

    -- Status
    active BOOLEAN DEFAULT true,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for Clients
CREATE INDEX idx_clients_tenant_id ON clients(tenant_id);
CREATE INDEX idx_clients_client_id ON clients(client_id);
CREATE INDEX idx_clients_active ON clients(active) WHERE deleted_at IS NULL;

COMMENT ON TABLE clients IS 'OAuth 2.0 clients - each client belongs to one tenant';
COMMENT ON COLUMN clients.google_oauth_enabled IS 'If true, use client-specific Google OAuth; if false, use Authway common OAuth';

-- ========================================
-- 5. Modify Sessions Table
-- ========================================

-- Rename user_sessions to sessions
ALTER TABLE IF EXISTS user_sessions RENAME TO sessions;

-- Add tenant_id to sessions for SSO logic
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Migrate existing sessions to default tenant
UPDATE sessions
SET tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL
ALTER TABLE sessions
    ALTER COLUMN tenant_id SET NOT NULL;

-- Add additional session metadata
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS ip_address INET,
    ADD COLUMN IF NOT EXISTS user_agent TEXT,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW();

-- Update indexes
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_id ON sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

COMMENT ON COLUMN sessions.tenant_id IS 'Tenant context for SSO - session valid only within same tenant';

-- ========================================
-- 6. Update Consent Grants Table
-- ========================================

-- Add tenant_id for better isolation
ALTER TABLE consent_grants
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Migrate existing grants to default tenant
UPDATE consent_grants
SET tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL
ALTER TABLE consent_grants
    ALTER COLUMN tenant_id SET NOT NULL;

-- Update indexes
CREATE INDEX IF NOT EXISTS idx_consent_grants_tenant_id ON consent_grants(tenant_id);

-- ========================================
-- 7. Create Email Verification Table
-- ========================================

CREATE TABLE IF NOT EXISTS email_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- User relationship
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Verification details
    email VARCHAR(255) NOT NULL,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Status
    verified_at TIMESTAMP WITH TIME ZONE,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_verifications_user_id ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_token ON email_verifications(token);
CREATE INDEX idx_email_verifications_expires_at ON email_verifications(expires_at);

-- ========================================
-- 8. Create Password Reset Table
-- ========================================

CREATE TABLE IF NOT EXISTS password_resets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- User relationship
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Reset details
    email VARCHAR(255) NOT NULL,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Status
    used_at TIMESTAMP WITH TIME ZONE,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_resets_user_id ON password_resets(user_id);
CREATE INDEX idx_password_resets_token ON password_resets(token);
CREATE INDEX idx_password_resets_expires_at ON password_resets(expires_at);

-- ========================================
-- 9. Create Demo Client for Default Tenant
-- ========================================

INSERT INTO clients (
    tenant_id,
    client_id,
    client_secret,
    name,
    description,
    redirect_uris,
    grant_types,
    scopes,
    active
)
VALUES (
    '00000000-0000-0000-0000-000000000001'::uuid,
    'demo-client-default',
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', -- bcrypt hash of 'demo-secret'
    'Demo Application',
    'Default demo client for testing',
    ARRAY['http://localhost:3000/callback', 'http://localhost:3001/callback'],
    ARRAY['authorization_code', 'refresh_token'],
    ARRAY['openid', 'profile', 'email'],
    true
)
ON CONFLICT (client_id) DO NOTHING;

-- ========================================
-- 10. Function: Update timestamp trigger
-- ========================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply triggers to tables with updated_at
DROP TRIGGER IF EXISTS update_tenants_updated_at ON tenants;
CREATE TRIGGER update_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_clients_updated_at ON clients;
CREATE TRIGGER update_clients_updated_at
    BEFORE UPDATE ON clients
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_sessions_updated_at ON sessions;
CREATE TRIGGER update_sessions_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ========================================
-- Migration Complete
-- ========================================

COMMIT;

-- Verification queries (run manually after migration)
-- SELECT COUNT(*) FROM tenants;
-- SELECT COUNT(*) FROM users WHERE tenant_id IS NOT NULL;
-- SELECT COUNT(*) FROM clients WHERE tenant_id IS NOT NULL;
-- SELECT email, tenant_id FROM users LIMIT 10;
