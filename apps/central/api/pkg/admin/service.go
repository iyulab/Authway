package admin

import (
	"crypto/subtle"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/tokenhash"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service interface {
	Authenticate(password string) (*AdminSession, error)
	ValidateToken(token string) (bool, error)
	Logout(token string) error
	CleanupExpiredSessions() error
}

type service struct {
	db       *gorm.DB
	logger   *zap.Logger
	password string // Admin password from config
}

func NewService(db *gorm.DB, logger *zap.Logger, adminPassword string) Service {
	return &service{
		db:       db,
		logger:   logger,
		password: adminPassword,
	}
}

// Authenticate validates admin password and creates session
func (s *service) Authenticate(password string) (*AdminSession, error) {
	// Constant-time compare to defend against timing oracles. Plain `!=` returns
	// as soon as a byte mismatches, leaking the matched-prefix length.
	if subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) != 1 {
		s.logger.Warn("Failed admin authentication attempt")
		return nil, fmt.Errorf("invalid password")
	}

	// Generate session token
	token, err := tokenhash.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Store only the hash; return the plaintext token to the caller
	session := &AdminSession{
		ID:        uuid.New(),
		TokenHash: tokenhash.Hash(token),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(session).Error; err != nil {
		s.logger.Error("Failed to create admin session", zap.Error(err))
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.Info("Admin authenticated successfully",
		zap.String("session_id", session.ID.String()))

	session.Token = token // in-memory only; handler returns this to the client
	return session, nil
}

// ValidateToken checks if token is valid and not expired
func (s *service) ValidateToken(token string) (bool, error) {
	var session AdminSession
	err := s.db.Where("token_hash = ? AND expires_at > ?", tokenhash.Hash(token), time.Now()).First(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to validate token: %w", err)
	}

	return true, nil
}

// Logout removes admin session
func (s *service) Logout(token string) error {
	result := s.db.Where("token_hash = ?", tokenhash.Hash(token)).Delete(&AdminSession{})
	if result.Error != nil {
		s.logger.Error("Failed to delete admin session", zap.Error(result.Error))
		return fmt.Errorf("failed to logout: %w", result.Error)
	}

	s.logger.Info("Admin logged out successfully")
	return nil
}

// CleanupExpiredSessions removes all expired sessions
func (s *service) CleanupExpiredSessions() error {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&AdminSession{})
	if result.Error != nil {
		s.logger.Error("Failed to cleanup expired sessions", zap.Error(result.Error))
		return fmt.Errorf("failed to cleanup: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.logger.Info("Cleaned up expired admin sessions",
			zap.Int64("count", result.RowsAffected))
	}

	return nil
}
