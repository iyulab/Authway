package impersonation

import (
	"time"

	"github.com/google/uuid"
)

// ImpersonationSession represents an active impersonation session
type ImpersonationSession struct {
	ID              uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null;index"`
	AdminID         uuid.UUID  `json:"admin_id" gorm:"type:uuid;not null;index"`
	AdminEmail      string     `json:"admin_email" gorm:"size:255;not null"`
	TargetUserID    uuid.UUID  `json:"target_user_id" gorm:"type:uuid;not null;index"`
	TargetUserEmail string     `json:"target_user_email" gorm:"size:255;not null"`
	Reason          string     `json:"reason" gorm:"type:text"`
	IPAddress       string     `json:"ip_address" gorm:"size:45"`
	UserAgent       string     `json:"user_agent" gorm:"size:512"`
	Token           string     `json:"-" gorm:"size:255;uniqueIndex"`
	Active          bool       `json:"active" gorm:"default:true"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	ExpiresAt       time.Time  `json:"expires_at" gorm:"not null"`
}

// IsExpired checks if the impersonation session has expired
func (s *ImpersonationSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// StartImpersonationRequest represents the request to start impersonation
type StartImpersonationRequest struct {
	TargetUserID uuid.UUID `json:"target_user_id" validate:"required"`
	Reason       string    `json:"reason" validate:"required,min=10"`
	Duration     int       `json:"duration"` // minutes, max 60
}

// ImpersonationTokenResponse represents the response with impersonation token
type ImpersonationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	TargetUser struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"target_user"`
}
