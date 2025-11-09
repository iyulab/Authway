-- Migration: Add user_claims table for dynamic claims management
-- Version: 001
-- Description: Adds support for dynamic user claims that can be injected into OAuth tokens

-- Create user_claims table
CREATE TABLE IF NOT EXISTS user_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    claim_key VARCHAR(255) NOT NULL,
    claim_value JSONB NOT NULL,
    is_permanent BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Foreign keys
    CONSTRAINT fk_user_claims_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_claims_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,

    -- Unique constraint: one claim key per user per tenant
    CONSTRAINT uq_user_claim_unique UNIQUE (user_id, tenant_id, claim_key)
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_user_claims_user_tenant ON user_claims(user_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_claims_key ON user_claims(claim_key);
CREATE INDEX IF NOT EXISTS idx_user_claims_permanent ON user_claims(is_permanent) WHERE is_permanent = true;

-- Create trigger to auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_user_claims_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_user_claims_updated_at
BEFORE UPDATE ON user_claims
FOR EACH ROW
EXECUTE FUNCTION update_user_claims_updated_at();

-- Add comment to table
COMMENT ON TABLE user_claims IS 'Stores dynamic user claims that can be added to OAuth tokens';
COMMENT ON COLUMN user_claims.id IS 'Unique identifier for the claim record';
COMMENT ON COLUMN user_claims.user_id IS 'Reference to the user who owns this claim';
COMMENT ON COLUMN user_claims.tenant_id IS 'Reference to the tenant context for this claim';
COMMENT ON COLUMN user_claims.claim_key IS 'The claim key (e.g., workspace_id, role, organization_id)';
COMMENT ON COLUMN user_claims.claim_value IS 'The claim value stored as JSONB for flexibility';
COMMENT ON COLUMN user_claims.is_permanent IS 'Whether this claim persists across sessions';
COMMENT ON COLUMN user_claims.created_at IS 'Timestamp when the claim was created';
COMMENT ON COLUMN user_claims.updated_at IS 'Timestamp when the claim was last updated';
