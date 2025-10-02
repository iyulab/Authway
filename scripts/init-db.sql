-- Initialize Authway database
-- This script creates the necessary database structure for Authway

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table for Authway user management
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    avatar_url VARCHAR(255),
    email_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- OAuth2 clients table for simplified client management
CREATE TABLE IF NOT EXISTS oauth_clients (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id VARCHAR(255) UNIQUE NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    client_secret_hash VARCHAR(255),
    redirect_uris TEXT[], -- Array of redirect URIs
    grant_types TEXT[] DEFAULT ARRAY['authorization_code', 'refresh_token'],
    response_types TEXT[] DEFAULT ARRAY['code'],
    scope TEXT DEFAULT 'openid profile email',
    owner_id UUID REFERENCES users(id) ON DELETE CASCADE,
    public BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- User sessions table for tracking active sessions
CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    session_token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_accessed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Consent grants table for user consent management
CREATE TABLE IF NOT EXISTS consent_grants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    client_id VARCHAR(255) NOT NULL,
    scope TEXT NOT NULL,
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_oauth_clients_client_id ON oauth_clients(client_id);
CREATE INDEX IF NOT EXISTS idx_oauth_clients_owner_id ON oauth_clients(owner_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_consent_grants_user_id ON consent_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_consent_grants_client_id ON consent_grants(client_id);

-- Insert default admin user (password: admin123)
INSERT INTO users (id, email, password_hash, name, email_verified)
VALUES (
    uuid_generate_v4(),
    'admin@authway.local',
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', -- bcrypt hash of 'admin123'
    'Admin User',
    TRUE
) ON CONFLICT (email) DO NOTHING;

-- Insert demo OAuth2 client
INSERT INTO oauth_clients (client_id, client_name, redirect_uris, owner_id)
VALUES (
    'demo-client',
    'Demo Application',
    ARRAY['http://localhost:3000/callback'],
    (SELECT id FROM users WHERE email = 'admin@authway.local' LIMIT 1)
) ON CONFLICT (client_id) DO NOTHING;