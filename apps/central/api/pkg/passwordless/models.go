package passwordless

import (
	"time"

	"github.com/google/uuid"
)

// MagicLinkTokenType represents the type of magic link
type MagicLinkTokenType string

const (
	TokenTypeLogin    MagicLinkTokenType = "login"
	TokenTypeRegister MagicLinkTokenType = "register"
	TokenTypeVerify   MagicLinkTokenType = "verify"
)

// MagicLink represents a magic link token for passwordless authentication
type MagicLink struct {
	ID        uuid.UUID          `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID  uuid.UUID          `json:"tenant_id" gorm:"type:uuid;not null;index"`
	Email     string             `json:"email" gorm:"size:255;not null;index"`
	Token     string             `json:"-" gorm:"size:255;not null;uniqueIndex"`
	TokenType MagicLinkTokenType `json:"token_type" gorm:"size:50;not null"`
	ClientID  string             `json:"client_id" gorm:"size:255"`
	RedirectURI string           `json:"redirect_uri" gorm:"size:2048"`
	State     string             `json:"state" gorm:"size:255"`
	IPAddress string             `json:"ip_address" gorm:"size:45"`
	UserAgent string             `json:"user_agent" gorm:"size:512"`
	UsedAt    *time.Time         `json:"used_at"`
	ExpiresAt time.Time          `json:"expires_at" gorm:"not null;index"`
	CreatedAt time.Time          `json:"created_at"`
}

// IsExpired checks if the magic link has expired
func (m *MagicLink) IsExpired() bool {
	return time.Now().After(m.ExpiresAt)
}

// IsUsed checks if the magic link has been used
func (m *MagicLink) IsUsed() bool {
	return m.UsedAt != nil
}

// SendMagicLinkRequest represents the request to send a magic link
type SendMagicLinkRequest struct {
	Email       string `json:"email" validate:"required,email"`
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
	State       string `json:"state"`
}

// VerifyMagicLinkRequest represents the request to verify a magic link
type VerifyMagicLinkRequest struct {
	Token string `json:"token" validate:"required"`
}

// MagicLinkResponse represents the response after sending a magic link
type MagicLinkResponse struct {
	Message   string    `json:"message"`
	ExpiresAt time.Time `json:"expires_at"`
}
