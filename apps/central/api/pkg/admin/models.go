package admin

import (
	"time"

	"github.com/google/uuid"
)

// AdminSession represents an admin console session
type AdminSession struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Token     string    `json:"-" gorm:"unique;not null"`
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
