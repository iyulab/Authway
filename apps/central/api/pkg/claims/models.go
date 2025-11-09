package claims

import (
	"time"

	"github.com/google/uuid"
)

// UserClaim represents a custom claim for a user
type UserClaim struct {
	ID          uuid.UUID              `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID      uuid.UUID              `json:"user_id" gorm:"type:uuid;not null;index:idx_user_claims_user_tenant"`
	TenantID    uuid.UUID              `json:"tenant_id" gorm:"type:uuid;not null;index:idx_user_claims_user_tenant"`
	ClaimKey    string                 `json:"claim_key" gorm:"type:varchar(255);not null;index:idx_user_claims_key;uniqueIndex:idx_user_claim_unique"`
	ClaimValue  map[string]interface{} `json:"claim_value" gorm:"type:jsonb;not null;serializer:json"`
	ClaimType   string                 `json:"claim_type" gorm:"type:varchar(50);not null;default:system;index:idx_user_claims_type"`
	IsPermanent bool                   `json:"is_permanent" gorm:"default:false"`
	CreatedAt   time.Time              `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time              `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName overrides the table name
func (UserClaim) TableName() string {
	return "user_claims"
}

// ClaimMap represents a map of claims (key-value pairs)
type ClaimMap map[string]interface{}

// UpdateClaimsRequest represents a request to update claims
type UpdateClaimsRequest struct {
	Claims      ClaimMap `json:"claims" validate:"required"`
	Permanent   bool     `json:"permanent"`
	ClientID    string   `json:"client_id" validate:"required"`
	RedirectURI string   `json:"redirect_uri" validate:"required,url"`
}

// UpdateClaimsResponse represents a response after updating claims
type UpdateClaimsResponse struct {
	Success bool   `json:"success"`
	AuthURL string `json:"auth_url"`
	Message string `json:"message"`
}

// GetClaimsResponse represents a response for get claims request
type GetClaimsResponse struct {
	UserID          uuid.UUID `json:"user_id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	Claims          ClaimMap  `json:"claims"`
	PermanentClaims []string  `json:"permanent_claims"`
	SessionClaims   []string  `json:"session_claims,omitempty"`
}

// DeleteClaimResponse represents a response for delete claim request
type DeleteClaimResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// UpdateUserClaimsRequest represents a request to update user-level claims (no re-auth required)
type UpdateUserClaimsRequest struct {
	Claims ClaimMap `json:"claims" validate:"required"`
}

// UpdateUserClaimsResponse represents a response after updating user claims
type UpdateUserClaimsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetUserClaimsResponse represents a response for get user claims request
type GetUserClaimsResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Claims   ClaimMap  `json:"claims"`
}
