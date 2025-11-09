package email

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailVerification represents an email verification token
type EmailVerification struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID      `json:"user_id" gorm:"type:uuid;index;not null"`
	Token     string         `json:"token" gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time      `json:"expires_at" gorm:"not null"`
	Verified  bool           `json:"verified" gorm:"default:false"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// BeforeCreate sets UUID and token if not provided
func (e *EmailVerification) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Token == "" {
		e.Token = uuid.New().String()
	}
	// Default expiration: 6 hours
	if e.ExpiresAt.IsZero() {
		e.ExpiresAt = time.Now().Add(6 * time.Hour)
	}
	return nil
}

// IsExpired checks if the verification token has expired
func (e *EmailVerification) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// PasswordReset represents a password reset token
type PasswordReset struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID      `json:"user_id" gorm:"type:uuid;index;not null"`
	Token     string         `json:"token" gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time      `json:"expires_at" gorm:"not null"`
	Used      bool           `json:"used" gorm:"default:false"`
	UsedAt    *time.Time     `json:"used_at"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// BeforeCreate sets UUID and token if not provided
func (p *PasswordReset) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.Token == "" {
		p.Token = uuid.New().String()
	}
	// Default expiration: 1 hour
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(1 * time.Hour)
	}
	return nil
}

// IsExpired checks if the reset token has expired
func (p *PasswordReset) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// IsValid checks if the token is valid (not used and not expired)
func (p *PasswordReset) IsValid() bool {
	return !p.Used && !p.IsExpired()
}

// MarkAsUsed marks the token as used
func (p *PasswordReset) MarkAsUsed() {
	now := time.Now()
	p.Used = true
	p.UsedAt = &now
}

// SendVerificationRequest represents a request to send verification email
type SendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// VerifyEmailRequest represents a request to verify email
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// ForgotPasswordRequest represents a request to reset password
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest represents a request to reset password with token
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

// VerifyResetTokenRequest represents a request to verify reset token
type VerifyResetTokenRequest struct {
	Token string `json:"token" validate:"required"`
}
