-- Migration: Add claim_type to user_claims table
-- Version: 003
-- Description: Distinguish between system claims (require re-auth) and user claims (no re-auth)

-- Add claim_type column
ALTER TABLE user_claims
ADD COLUMN IF NOT EXISTS claim_type VARCHAR(50) DEFAULT 'system' NOT NULL;

-- Create index for efficient filtering by claim type
CREATE INDEX IF NOT EXISTS idx_user_claims_type ON user_claims(claim_type);

-- Update existing claims to be 'system' type (default behavior)
UPDATE user_claims SET claim_type = 'system' WHERE claim_type IS NULL;

-- Add comment
COMMENT ON COLUMN user_claims.claim_type IS 'Type of claim: system (requires re-auth) or user (no re-auth)';
