package invitation

import (
	"time"

	"github.com/google/uuid"
)

// InvitationStatus represents the status of an invitation
type InvitationStatus string

const (
	StatusPending  InvitationStatus = "pending"
	StatusAccepted InvitationStatus = "accepted"
	StatusDeclined InvitationStatus = "declined"
	StatusExpired  InvitationStatus = "expired"
	StatusRevoked  InvitationStatus = "revoked"
)

// Invitation represents an organization/tenant invitation
type Invitation struct {
	ID          uuid.UUID        `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID        `json:"tenant_id" gorm:"type:uuid;not null;index"`
	TenantName  string           `json:"tenant_name" gorm:"size:255"`
	InviterID   uuid.UUID        `json:"inviter_id" gorm:"type:uuid;not null"`
	InviterName string           `json:"inviter_name" gorm:"size:255"`
	Email       string           `json:"email" gorm:"size:255;not null;index"`
	Role        string           `json:"role" gorm:"size:50;default:member"`
	Token       string           `json:"-" gorm:"size:255;uniqueIndex"`
	Status      InvitationStatus `json:"status" gorm:"size:20;default:pending;index"`
	Message     string           `json:"message" gorm:"type:text"`
	AcceptedAt  *time.Time       `json:"accepted_at"`
	AcceptedBy  *uuid.UUID       `json:"accepted_by" gorm:"type:uuid"`
	ExpiresAt   time.Time        `json:"expires_at" gorm:"not null"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// IsExpired checks if the invitation has expired
func (i *Invitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// CanBeAccepted checks if the invitation can still be accepted
func (i *Invitation) CanBeAccepted() bool {
	return i.Status == StatusPending && !i.IsExpired()
}

// CreateInvitationRequest represents the request to create an invitation
type CreateInvitationRequest struct {
	Email   string `json:"email" validate:"required,email"`
	Role    string `json:"role"`
	Message string `json:"message"`
}

// AcceptInvitationRequest represents the request to accept an invitation
type AcceptInvitationRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password"` // optional if user already exists
	Name     string `json:"name"`     // optional if user already exists
}

// InvitationResponse represents the invitation details response
type InvitationResponse struct {
	ID         string `json:"id"`
	TenantName string `json:"tenant_name"`
	InviterName string `json:"inviter_name"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	Message    string `json:"message"`
	ExpiresAt  time.Time `json:"expires_at"`
}
