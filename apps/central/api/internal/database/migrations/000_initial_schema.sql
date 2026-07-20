-- ============================================================
-- 000: Initial schema
-- ============================================================
-- Creates the base tables every later migration builds on.
--
-- The whole file is wrapped in a guard: if `tenants` already exists, nothing
-- runs. That makes it a no-op on any database that already has a schema —
-- which is the property that keeps existing deployments safe, and it is a
-- structural guarantee rather than a per-statement one.
--
-- Per-statement `IF NOT EXISTS` is NOT sufficient here and was tried first.
-- Later migrations reshape these tables (013/014 drop `token` in favour of
-- `token_hash`), so re-running the 000-era `CREATE INDEX ... (token)` against a
-- current database fails with `column "token" does not exist`. Any future
-- migration that drops a column would reopen the same hole. One guard on "is
-- there a schema at all" closes it permanently.
--
-- History: this replaced 000_v0_clean_slate.sql, whose first act was
-- `DROP TABLE ... CASCADE`. That file was never actually executed — its version
-- collided with a bookkeeping sentinel row, so the migrator always considered it
-- applied, which is the only reason no deployment was ever wiped, and also why
-- no blank database could be provisioned. The destructive script now lives at
-- scripts/bootstrap/dev-clean-slate.sql and is only ever run by hand.
--
-- Deliberately contains NO `BEGIN;`/`COMMIT;`: RunMigrations owns a single outer
-- transaction for the whole run, and a nested COMMIT would commit it early,
-- destroying the all-or-nothing guarantee.
--
-- Only base tables belong here. Columns added later (claims, MFA, consent flags,
-- token hashing, …) stay in their own migrations — this file must keep
-- describing the schema as it was at 000, or a fresh database and an existing
-- one would diverge.
-- ============================================================

DO $do$
BEGIN

IF to_regclass('public.tenants') IS NOT NULL THEN
    RAISE NOTICE '000_initial_schema: schema already present, skipping';
    RETURN;
END IF;

-- ============================================================
-- 1. Tenants (base isolation unit)
-- ============================================================

CREATE TABLE tenants (
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

INSERT INTO tenants (name, slug, description, active)
VALUES ('Default Tenant', 'default', 'Default tenant for multi-tenant mode', true);

-- ============================================================
-- 2. Users (tenant-scoped from the start)
-- ============================================================

CREATE TABLE users (
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
-- 3. Clients (OAuth 2.0 clients, tenant-scoped)
-- ============================================================

CREATE TABLE clients (
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
-- 4. Sessions (tenant_id carried for SSO verification)
-- ============================================================

CREATE TABLE sessions (
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
-- 5. Email verification
-- ============================================================

CREATE TABLE email_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    verified BOOLEAN DEFAULT false,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_email_verifications_user ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_token ON email_verifications(token);

COMMENT ON TABLE email_verifications IS 'Email verification tokens for user registration';

-- ============================================================
-- 6. Password reset
-- ============================================================

CREATE TABLE password_resets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    used BOOLEAN DEFAULT false,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_password_resets_user ON password_resets(user_id);
CREATE INDEX idx_password_resets_token ON password_resets(token);

COMMENT ON TABLE password_resets IS 'Password reset tokens for user recovery';

-- ============================================================
-- 7. Admin sessions
-- ============================================================

CREATE TABLE admin_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_sessions_token ON admin_sessions(token);
CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);

COMMENT ON TABLE admin_sessions IS 'Admin console session tokens';
COMMENT ON COLUMN admin_sessions.token IS 'Session token for admin console authentication';
COMMENT ON COLUMN admin_sessions.expires_at IS 'Token expiration time (24 hours default)';

-- ============================================================
-- 8. updated_at trigger
-- ============================================================

-- The function body uses its own dollar-quote tag, distinct from the one that
-- opens the guard above. Reusing that tag anywhere in here (even inside a
-- comment — dollar quoting is lexical and ignores comment syntax) would close
-- the guard early and leave the rest of the file as bare, broken SQL.
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $fn$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$fn$ LANGUAGE plpgsql;

CREATE TRIGGER update_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_clients_updated_at BEFORE UPDATE ON clients
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sessions_updated_at BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

END
$do$;
