-- Migration: Add authentication providers configuration
-- Version: 007
-- Date: 2025-12-07
-- Description: Add client-level auth provider selection and email signup settings

BEGIN;

-- ============================================================
-- 1. Add authentication provider settings to clients table
-- ============================================================

-- Enabled authentication providers for this client
-- Stored as array of provider names: ['email', 'google', 'github', 'microsoft', 'apple']
ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS enabled_auth_providers TEXT[] DEFAULT '{email,google}';

-- Whether direct email/password signup is allowed
ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS allow_email_signup BOOLEAN DEFAULT true;

-- Whether direct email/password login is allowed
ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS allow_email_login BOOLEAN DEFAULT true;

-- Microsoft OAuth settings (if client wants to use their own credentials)
ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS microsoft_oauth_enabled BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS microsoft_client_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS microsoft_client_secret TEXT,
    ADD COLUMN IF NOT EXISTS microsoft_tenant_id VARCHAR(255);

-- Apple OAuth settings (if client wants to use their own credentials)
ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS apple_oauth_enabled BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS apple_client_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_team_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_key_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_private_key TEXT;

-- ============================================================
-- 2. Create system configuration table for global settings
-- ============================================================

CREATE TABLE IF NOT EXISTS system_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(100) UNIQUE NOT NULL,
    value JSONB NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_config_key ON system_config(key);

-- Insert default available auth providers configuration
INSERT INTO system_config (key, value, description)
VALUES (
    'available_auth_providers',
    '["email", "google"]'::jsonb,
    'List of authentication providers available system-wide. Clients can only enable providers from this list.'
)
ON CONFLICT (key) DO NOTHING;

-- Insert default auth provider settings
INSERT INTO system_config (key, value, description)
VALUES (
    'auth_provider_settings',
    '{
        "email": {
            "enabled": true,
            "allow_signup": true,
            "require_email_verification": true
        },
        "google": {
            "enabled": true,
            "client_id": "",
            "client_secret": ""
        },
        "github": {
            "enabled": false,
            "client_id": "",
            "client_secret": ""
        },
        "microsoft": {
            "enabled": false,
            "client_id": "",
            "client_secret": "",
            "tenant_id": "common"
        },
        "apple": {
            "enabled": false,
            "client_id": "",
            "team_id": "",
            "key_id": "",
            "private_key": ""
        }
    }'::jsonb,
    'Global settings for each authentication provider. Only providers with enabled=true will be available.'
)
ON CONFLICT (key) DO NOTHING;

-- ============================================================
-- 3. Add comments
-- ============================================================

COMMENT ON COLUMN clients.enabled_auth_providers IS 'Array of enabled auth provider names for this client. Default: [email, google]';
COMMENT ON COLUMN clients.allow_email_signup IS 'Whether users can register with email/password. Default: true';
COMMENT ON COLUMN clients.allow_email_login IS 'Whether users can login with email/password. Default: true';

COMMENT ON TABLE system_config IS 'System-wide configuration storage in JSONB format';
COMMENT ON COLUMN system_config.key IS 'Unique configuration key (e.g., available_auth_providers)';
COMMENT ON COLUMN system_config.value IS 'Configuration value in JSONB format';

-- ============================================================
-- 4. Update trigger
-- ============================================================

CREATE TRIGGER update_system_config_updated_at BEFORE UPDATE ON system_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMIT;

-- ============================================================
-- Migration Complete
-- ============================================================
--
-- New columns in clients table:
-- - enabled_auth_providers: TEXT[] (default: {email,google})
-- - allow_email_signup: BOOLEAN (default: true)
-- - allow_email_login: BOOLEAN (default: true)
-- - microsoft_oauth_enabled, microsoft_client_id, etc.
-- - apple_oauth_enabled, apple_client_id, etc.
--
-- New table:
-- - system_config: Key-value store for system-wide settings
--
-- Verification:
-- SELECT * FROM system_config;
-- SELECT enabled_auth_providers, allow_email_signup FROM clients LIMIT 1;
