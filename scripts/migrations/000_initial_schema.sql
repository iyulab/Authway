-- Initial Database Schema
-- This creates the base tables for Authway with multi-tenancy support from the start

BEGIN;

-- ============================================================
-- 1. Tenants Table (Base isolation unit)
-- ============================================================

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    settings JSONB DEFAULT '{
        "require_email_verification": true,
        "password_min_length": 8,
        "session_timeout": 60,
        "allowed_domains": []
    }'::jsonb,
    logo TEXT,
    primary_color VARCHAR(20) DEFAULT '#4F46E5',
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_tenants_slug ON tenants(slug);
CREATE INDEX idx_tenants_active ON tenants(active);

COMMENT ON TABLE tenants IS 'Multi-tenant isolation boundary. Each tenant represents a separate organization or application.';
COMMENT ON COLUMN tenants.slug IS 'URL-friendly unique identifier for tenant';
COMMENT ON COLUMN tenants.settings IS 'Tenant-specific configuration (email verification, password policy, session timeout, etc.)';

-- Insert default tenant
INSERT INTO tenants (name, slug, description, active)
VALUES ('Default Tenant', 'default', 'Default tenant for multi-tenant mode', true)
ON CONFLICT (slug) DO NOTHING;

-- ============================================================
-- 2. Users Table (with tenant_id from the start)
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    password_hash TEXT,
    name VARCHAR(255),
    avatar_url TEXT,
    email_verified BOOLEAN DEFAULT false,
    active BOOLEAN DEFAULT true,
    provider VARCHAR(50) DEFAULT 'local',
    google_id VARCHAR(255),
    github_id VARCHAR(255),
    picture TEXT,
    last_login_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Composite unique index: same email can exist in different tenants
CREATE UNIQUE INDEX idx_users_tenant_email ON users(tenant_id, email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_google_id ON users(google_id);
CREATE INDEX idx_users_github_id ON users(github_id);

COMMENT ON TABLE users IS 'User accounts isolated by tenant. Same email can exist in different tenants.';
COMMENT ON COLUMN users.tenant_id IS 'Tenant isolation - users belong to exactly one tenant';
COMMENT ON COLUMN users.provider IS 'Authentication provider: local, google, github';

-- ============================================================
-- 3. Clients Table (OAuth 2.0 clients with tenant_id)
-- ============================================================

CREATE TABLE IF NOT EXISTS clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    client_id VARCHAR(255) UNIQUE NOT NULL,
    client_secret TEXT NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    website TEXT,
    logo TEXT,
    redirect_uris TEXT[] NOT NULL DEFAULT '{}',
    grant_types TEXT[] NOT NULL DEFAULT '{}',
    scopes TEXT[] NOT NULL DEFAULT '{}',
    public BOOLEAN DEFAULT false,
    active BOOLEAN DEFAULT true,

    -- Client-specific Google OAuth
    google_oauth_enabled BOOLEAN DEFAULT false,
    google_client_id VARCHAR(255),
    google_client_secret TEXT,
    google_redirect_uri TEXT,

    -- Client-specific GitHub OAuth
    github_oauth_enabled BOOLEAN DEFAULT false,
    github_client_id VARCHAR(255),
    github_client_secret TEXT,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_clients_tenant ON clients(tenant_id);
CREATE INDEX idx_clients_client_id ON clients(client_id);
CREATE INDEX idx_clients_active ON clients(active);

COMMENT ON TABLE clients IS 'OAuth 2.0 clients isolated by tenant. Each client belongs to one tenant.';
COMMENT ON COLUMN clients.tenant_id IS 'Tenant ownership - SSO works only within same tenant';
COMMENT ON COLUMN clients.google_oauth_enabled IS 'If true, use client-specific Google OAuth; otherwise use Authway common settings';
COMMENT ON COLUMN clients.github_oauth_enabled IS 'If true, use client-specific GitHub OAuth; otherwise use Authway common settings';

-- ============================================================
-- 4. Sessions Table (with tenant_id for SSO verification)
-- ============================================================

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_tenant ON sessions(tenant_id);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_token ON sessions(token);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

COMMENT ON TABLE sessions IS 'User sessions with tenant_id for SSO verification';
COMMENT ON COLUMN sessions.tenant_id IS 'Tenant context - SSO check: session.tenant_id == client.tenant_id';

-- ============================================================
-- 5. Email Verification Table
-- ============================================================

CREATE TABLE IF NOT EXISTS email_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_verifications_user ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_token ON email_verifications(token);

-- ============================================================
-- 6. Password Reset Table
-- ============================================================

CREATE TABLE IF NOT EXISTS password_resets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_resets_user ON password_resets(user_id);
CREATE INDEX idx_password_resets_token ON password_resets(token);

-- ============================================================
-- 7. Trigger for updated_at
-- ============================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_clients_updated_at BEFORE UPDATE ON clients
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sessions_updated_at BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMIT;
