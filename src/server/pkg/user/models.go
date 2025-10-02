package user

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primary_key"`
	Email         string         `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash  string         `json:"-" gorm:"not null"` // Match SQL schema
	Name          *string        `json:"name"`              // Match SQL schema - single name field
	AvatarURL     *string        `json:"avatar_url"`        // Match SQL schema column name
	EmailVerified bool           `json:"email_verified" gorm:"default:false"`
	Active        bool           `json:"active" gorm:"default:true"`
	Provider      string         `json:"provider" gorm:"default:local"` // local, google, github, etc.
	GoogleID      *string        `json:"-" gorm:"index"`                // Google user ID
	Picture       *string        `json:"picture"`                       // Profile picture URL
	LastLoginAt   *time.Time     `json:"last_login_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at" gorm:"index"`
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
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	AvatarURL     string     `json:"avatar_url"`
	EmailVerified bool       `json:"email_verified"`
	Active        bool       `json:"active"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
		Email:         u.Email,
		Name:          name,
		AvatarURL:     avatarURL,
		EmailVerified: u.EmailVerified,
		Active:        u.Active,
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

// UpdateUserRequest represents the request to update a user
type UpdateUserRequest struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// LoginRequest represents the login request
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// ChangePasswordRequest represents the change password request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}
