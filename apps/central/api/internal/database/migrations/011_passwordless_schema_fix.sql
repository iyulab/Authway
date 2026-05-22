-- Migration: Fix magic_link_tokens schema to match MagicLink model
-- Version: 011
-- Date: 2026-05-22

-- magic_link_tokens was created in 006 with a minimal schema.
-- MagicLink model has expanded fields; add missing columns.

ALTER TABLE magic_link_tokens
    ADD COLUMN IF NOT EXISTS token_type  VARCHAR(50) NOT NULL DEFAULT 'login',
    ADD COLUMN IF NOT EXISTS client_id   VARCHAR(255),
    ADD COLUMN IF NOT EXISTS redirect_uri VARCHAR(2048),
    ADD COLUMN IF NOT EXISTS state       VARCHAR(255),
    ADD COLUMN IF NOT EXISTS ip_address  VARCHAR(45),
    ADD COLUMN IF NOT EXISTS user_agent  VARCHAR(512),
    ADD COLUMN IF NOT EXISTS used_at     TIMESTAMP WITH TIME ZONE;

-- Backfill used_at from existing used boolean (one-time)
UPDATE magic_link_tokens SET used_at = NOW() WHERE used = true AND used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_magic_link_token_type ON magic_link_tokens(token_type);
CREATE INDEX IF NOT EXISTS idx_magic_link_used_at    ON magic_link_tokens(used_at);
