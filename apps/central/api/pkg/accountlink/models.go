package accountlink

import (
	"time"

	"github.com/google/uuid"
)

// Provider represents the type of social login provider
type Provider string

const (
	ProviderGoogle    Provider = "google"
	ProviderGitHub    Provider = "github"
	ProviderMicrosoft Provider = "microsoft"
	ProviderApple     Provider = "apple"
)

// LinkedAccount represents a social account linked to a user
type LinkedAccount struct {
	ID           uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID       uuid.UUID  `json:"user_id" gorm:"type:uuid;not null;index"`
	TenantID     uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null;index"`
	Provider     Provider   `json:"provider" gorm:"size:50;not null;index"`
	ProviderID   string     `json:"provider_id" gorm:"size:255;not null"`
	Email        string     `json:"email" gorm:"size:255"`
	Name         string     `json:"name" gorm:"size:255"`
	AvatarURL    string     `json:"avatar_url" gorm:"size:512"`
	AccessToken  string     `json:"-" gorm:"type:text"`
	RefreshToken string     `json:"-" gorm:"type:text"`
	TokenExpiry  *time.Time `json:"token_expiry"`
	Metadata     string     `json:"-" gorm:"type:text"` // JSON metadata
	LinkedAt     time.Time  `json:"linked_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName specifies the database table name
func (LinkedAccount) TableName() string {
	return "linked_accounts"
}

// LinkAccountRequest represents the request to link a social account
type LinkAccountRequest struct {
	Provider    Provider `json:"provider" validate:"required"`
	Code        string   `json:"code" validate:"required"`
	RedirectURI string   `json:"redirect_uri"`
	State       string   `json:"state"`
}

// UnlinkAccountRequest represents the request to unlink a social account
type UnlinkAccountRequest struct {
	Provider Provider `json:"provider" validate:"required"`
}

// LinkedAccountResponse represents the response for a linked account
type LinkedAccountResponse struct {
	ID         string     `json:"id"`
	Provider   Provider   `json:"provider"`
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	AvatarURL  string     `json:"avatar_url,omitempty"`
	LinkedAt   time.Time  `json:"linked_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ToResponse converts LinkedAccount to LinkedAccountResponse
func (l *LinkedAccount) ToResponse() *LinkedAccountResponse {
	return &LinkedAccountResponse{
		ID:         l.ID.String(),
		Provider:   l.Provider,
		Email:      l.Email,
		Name:       l.Name,
		AvatarURL:  l.AvatarURL,
		LinkedAt:   l.LinkedAt,
		LastUsedAt: l.LastUsedAt,
	}
}

// AvailableProvidersResponse represents the available providers for linking
type AvailableProvidersResponse struct {
	Provider    Provider `json:"provider"`
	DisplayName string   `json:"display_name"`
	AuthURL     string   `json:"auth_url,omitempty"`
	IsLinked    bool     `json:"is_linked"`
}
