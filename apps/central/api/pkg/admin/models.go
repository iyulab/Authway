package admin

import (
	"time"

	"github.com/google/uuid"
)

// AdminSession represents an admin console session.
// TokenHash is stored in DB; Token is populated in-memory only after Authenticate.
type AdminSession struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	TokenHash string    `json:"-" gorm:"column:token_hash;unique;not null"`
	Token     string    `json:"-" gorm:"-"` // plaintext; returned to client on login, never persisted
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginRequest for admin console authentication
type LoginRequest struct {
	Password string `json:"password" validate:"required"`
}

// LoginResponse returns session token
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AdminInfo returns admin console information
type AdminInfo struct {
	Authenticated bool   `json:"authenticated"`
	Version       string `json:"version"`
}
