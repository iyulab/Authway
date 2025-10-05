-- Add admin sessions table for admin console authentication

BEGIN;

CREATE TABLE IF NOT EXISTS admin_sessions (
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

COMMIT;
