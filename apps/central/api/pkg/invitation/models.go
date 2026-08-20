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

// SystemInviterName is the display name used when an invitation was created by
// the system actor (admin API key) rather than by a signed-in user. Such an
// invitation has a NULL inviter_id — there is no user row behind it.
const SystemInviterName = "system"

// Invitation represents an organization/tenant invitation.
//
// Column mapping is authoritative in migrations/006 (+ 016); the struct follows
// it. TenantName/InviterName are NOT columns — they are derived at read time
// from tenants/users, so the invitation row never carries a second, drifting
// copy of a name that lives elsewhere.
type Invitation struct {
	ID       uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID uuid.UUID `json:"tenant_id" gorm:"type:uuid;not null;index"`
	// InviterID is NULL for invitations created by the system actor (admin API
	// key). The FK is ON DELETE SET NULL, so deleting a user orphans their
	// invitations rather than destroying them.
	InviterID  *uuid.UUID       `json:"inviter_id" gorm:"type:uuid"`
	Email      string           `json:"email" gorm:"size:255;not null;index"`
	Role       string           `json:"role" gorm:"size:50;default:member"`
	// TokenHash is the SHA-256 hex digest of the invitation token. The
	// plaintext never touches the database — an invitation token grants
	// account-creation capability on its own, so a DB read must not yield a
	// usable one. See migration 020 and BackfillTokenHashes.
	TokenHash  string           `json:"-" gorm:"column:token_hash;size:64;not null;uniqueIndex"`
	Status     InvitationStatus `json:"status" gorm:"size:20;default:pending;index"`
	Message    string           `json:"message" gorm:"type:text"`
	AcceptedAt *time.Time       `json:"accepted_at"`
	AcceptedBy *uuid.UUID       `json:"accepted_by" gorm:"column:accepted_user_id;type:uuid"`
	ExpiresAt  time.Time        `json:"expires_at" gorm:"not null"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`

	// Derived, never persisted — hydrated by the service on read.
	TenantName  string `json:"tenant_name" gorm:"-"`
	InviterName string `json:"inviter_name" gorm:"-"`
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
