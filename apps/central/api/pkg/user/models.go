package user

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a user in the system
// Users are isolated by tenant - same email can exist in different tenants
type User struct {
	ID            uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID      uuid.UUID `json:"tenant_id" gorm:"type:uuid;not null;index:idx_users_tenant_email"`
	Email         string    `json:"email" gorm:"not null;index:idx_users_tenant_email;index"`
	PasswordHash  string    `json:"-" gorm:"not null"`
	Name          *string   `json:"name"`
	AvatarURL     *string   `json:"avatar_url"`
	EmailVerified bool      `json:"email_verified" gorm:"default:false"`
	// Active is false while an administrator has suspended the account: it
	// cannot sign in (see the sign-in steps) and its sessions were ended.
	Active bool `json:"active"`
	// The account at each sign-in provider this account signs in with. Within
	// a tenant a provider account belongs to one account that is not deleted
	// (unique indexes uq_users_tenant_<provider>_id).
	GoogleID    *string `json:"-"`
	GithubID    *string `json:"-"`
	MicrosoftID *string `json:"-"`
	AppleID     *string `json:"-"`
	Picture     *string `json:"picture"`

	// MFA/TOTP fields
	TOTPSecret     *string    `json:"-" gorm:"column:totp_secret"`
	TOTPEnabled    bool       `json:"totp_enabled" gorm:"default:false"`
	TOTPVerifiedAt *time.Time `json:"totp_verified_at"`
	// TOTPLastStep is the time step of the last authenticator code accepted,
	// so that a code is accepted once (RFC 6238 section 5.2).
	TOTPLastStep  *int64  `json:"-" gorm:"column:totp_last_step"`
	RecoveryCodes *string `json:"-" gorm:"column:recovery_codes"` // JSON array of hashed codes

	LastLoginAt *time.Time     `json:"last_login_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// BeforeCreate sets UUID if not provided
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

// PublicUser returns user data safe for public consumption
type PublicUser struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	AvatarURL     string    `json:"avatar_url"`
	EmailVerified bool      `json:"email_verified"`
	Active        bool      `json:"active"`
	// Connections are the sign-in providers the account is linked to. (The
	// stored provider column said "local" for every account — nothing set it.)
	Connections []string   `json:"connections"`
	TOTPEnabled bool       `json:"totp_enabled"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ToPublic converts User to PublicUser
func (u *User) ToPublic() PublicUser {
	var name, avatarURL string
	if u.Name != nil {
		name = *u.Name
	}
	if u.AvatarURL != nil {
		avatarURL = *u.AvatarURL
	}

	return PublicUser{
		ID:            u.ID,
		TenantID:      u.TenantID,
		Email:         u.Email,
		Name:          name,
		AvatarURL:     avatarURL,
		EmailVerified: u.EmailVerified,
		Active:        u.Active,
		Connections:   append([]string{}, u.LinkedProviders()...),
		TOTPEnabled:   u.TOTPEnabled,
		LastLoginAt:   u.LastLoginAt,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

// CreateUserRequest represents the request to create a new user
type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"omitempty,min=8"` // Optional for social login
	Name     string `json:"name" validate:"required"`
}

// UpdateUserRequest changes a user's profile: a field left out stays as it
// is, and an empty one is cleared.
type UpdateUserRequest struct {
	Name      *string `json:"name"`
	AvatarURL *string `json:"avatar_url"`
	// Active false suspends the account: it cannot sign in, and its sessions
	// end. True lifts the suspension. Left out, it stays as it is.
	Active *bool `json:"active"`
}

// LoginRequest represents the login request
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginWithMFARequest represents login with MFA code
type LoginWithMFARequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	TOTPCode string `json:"totp_code" validate:"required,len=6"`
}

// ChangePasswordRequest is a signed-in user setting a new password. An
// account that has one proves it with CurrentPassword; an account without
// one (signed in by link or by another provider) sends none. How long the new
// one must be is the tenant's rule (see checkPassword).
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
