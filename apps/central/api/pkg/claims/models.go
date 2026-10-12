package claims

import (
	"time"

	"github.com/google/uuid"
)

// UserClaim is one claim stored for a user.
type UserClaim struct {
	ID         uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID      `json:"user_id" gorm:"type:uuid;not null;index:idx_user_claims_user_tenant"`
	TenantID   uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index:idx_user_claims_user_tenant"`
	ClaimKey   string         `json:"claim_key" gorm:"type:varchar(255);not null;index:idx_user_claims_key;uniqueIndex:idx_user_claim_unique"`
	ClaimValue map[string]any `json:"claim_value" gorm:"type:jsonb;not null;serializer:json"`
	CreatedAt  time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `json:"updated_at" gorm:"autoUpdateTime"`

	// ClaimType and IsPermanent told apart kinds of claims that no longer
	// exist: every stored claim is written by a trusted party and kept until
	// removed. Nothing writes or reads them (the columns take their
	// defaults); they stay mapped, read-only, until a migration drops them.
	// TODO: drop user_claims.claim_type and is_permanent, then these fields.
	ClaimType   string `json:"-" gorm:"->;type:varchar(50);not null;default:system;index:idx_user_claims_type"`
	IsPermanent bool   `json:"-" gorm:"->;default:false"`
}

// TableName overrides the table name
func (UserClaim) TableName() string {
	return "user_claims"
}

// value is the claim's value: stored claims wrap it as {"value": …}.
func (c UserClaim) value() any {
	if v, ok := c.ClaimValue["value"]; ok {
		return v
	}
	return c.ClaimValue
}

// ClaimMap represents a map of claims (key-value pairs)
type ClaimMap map[string]any

// PatchClaimsRequest is the body of PATCH /api/v1/users/{id}/claims: a JSON
// merge patch (RFC 7396) of the user's claims — a value sets the claim, null
// removes it, a claim not named is left alone.
type PatchClaimsRequest struct {
	Claims map[string]any `json:"claims"`
}

// ClaimsResponse is the user's stored claims.
type ClaimsResponse struct {
	Claims ClaimMap `json:"claims"`
}
