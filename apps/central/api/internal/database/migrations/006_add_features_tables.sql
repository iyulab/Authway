-- Migration: Add Webhooks, Audit Logs, Invitations, and Impersonation tables
-- Version: 006
-- Date: 2025-12-07

-- ============================================================
-- 1. Webhooks Table
-- ============================================================

CREATE TABLE IF NOT EXISTS webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    url TEXT NOT NULL,
    events TEXT[] NOT NULL DEFAULT '{}',
    enabled BOOLEAN DEFAULT true,
    secret TEXT NOT NULL,
    retry_count INTEGER DEFAULT 3,
    timeout_secs INTEGER DEFAULT 30,
    last_triggered_at TIMESTAMP WITH TIME ZONE,
    last_status_code INTEGER,
    last_error TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_webhooks_tenant ON webhooks(tenant_id);
CREATE INDEX idx_webhooks_enabled ON webhooks(enabled);

COMMENT ON TABLE webhooks IS 'Webhook configurations for event notifications';
COMMENT ON COLUMN webhooks.events IS 'Array of event types to trigger webhook (e.g., user.created, user.login)';
COMMENT ON COLUMN webhooks.secret IS 'Shared secret for HMAC-SHA256 signature verification';

-- ============================================================
-- 2. Webhook Deliveries Table (Delivery History)
-- ============================================================

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status_code INTEGER,
    response_body TEXT,
    error TEXT,
    duration_ms INTEGER,
    attempt INTEGER DEFAULT 1,
    delivered_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_event ON webhook_deliveries(event_type);
CREATE INDEX idx_webhook_deliveries_delivered ON webhook_deliveries(delivered_at);

COMMENT ON TABLE webhook_deliveries IS 'Webhook delivery history and attempts';

-- ============================================================
-- 3. Audit Logs Table
-- ============================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_email VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255),
    description TEXT,
    ip_address VARCHAR(45),
    user_agent TEXT,
    severity VARCHAR(20) DEFAULT 'info',
    success BOOLEAN DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_tenant ON audit_logs(tenant_id);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at);
CREATE INDEX idx_audit_logs_severity ON audit_logs(severity);

COMMENT ON TABLE audit_logs IS 'Comprehensive audit trail for all system activities';
COMMENT ON COLUMN audit_logs.severity IS 'info, warning, error, critical';
COMMENT ON COLUMN audit_logs.metadata IS 'Additional context-specific data in JSON format';

-- ============================================================
-- 4. Invitations Table
-- ============================================================

CREATE TABLE IF NOT EXISTS invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    inviter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    message TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    accepted_at TIMESTAMP WITH TIME ZONE,
    accepted_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_invitations_tenant ON invitations(tenant_id);
CREATE INDEX idx_invitations_email ON invitations(email);
CREATE INDEX idx_invitations_token ON invitations(token);
CREATE INDEX idx_invitations_status ON invitations(status);
CREATE INDEX idx_invitations_expires ON invitations(expires_at);

COMMENT ON TABLE invitations IS 'User invitation management for tenant onboarding';
COMMENT ON COLUMN invitations.status IS 'pending, accepted, expired, revoked';
COMMENT ON COLUMN invitations.role IS 'Role to assign when invitation is accepted';

-- ============================================================
-- 5. Impersonation Sessions Table
-- ============================================================

CREATE TABLE IF NOT EXISTS impersonation_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    active BOOLEAN DEFAULT true,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMP WITH TIME ZONE,
    ip_address VARCHAR(45),
    user_agent TEXT
);

CREATE INDEX idx_impersonation_admin ON impersonation_sessions(admin_id);
CREATE INDEX idx_impersonation_target ON impersonation_sessions(target_user_id);
CREATE INDEX idx_impersonation_token ON impersonation_sessions(token);
CREATE INDEX idx_impersonation_active ON impersonation_sessions(active);

COMMENT ON TABLE impersonation_sessions IS 'Admin user impersonation sessions with audit trail';
COMMENT ON COLUMN impersonation_sessions.reason IS 'Required reason for impersonation (compliance)';

-- ============================================================
-- 6. Magic Link Tokens Table
-- ============================================================

CREATE TABLE IF NOT EXISTS magic_link_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    login_challenge TEXT,
    used BOOLEAN DEFAULT false,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_magic_link_token ON magic_link_tokens(token);
CREATE INDEX idx_magic_link_email ON magic_link_tokens(email);
CREATE INDEX idx_magic_link_expires ON magic_link_tokens(expires_at);

COMMENT ON TABLE magic_link_tokens IS 'Passwordless authentication tokens';
COMMENT ON COLUMN magic_link_tokens.login_challenge IS 'Hydra login challenge for OAuth flow continuation';

-- ============================================================
-- 7. Update Triggers
-- ============================================================

CREATE TRIGGER update_webhooks_updated_at BEFORE UPDATE ON webhooks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_invitations_updated_at BEFORE UPDATE ON invitations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- Migration Complete
-- ============================================================
--
-- New tables:
-- - webhooks: Webhook configuration and delivery tracking
-- - webhook_deliveries: Webhook delivery history
-- - audit_logs: Comprehensive audit trail
-- - invitations: User invitation management
-- - impersonation_sessions: Admin impersonation tracking
-- - magic_link_tokens: Passwordless authentication
--
-- Verification queries:
-- SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;
