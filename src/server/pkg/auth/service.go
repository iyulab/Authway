package auth

import (
	"fmt"

	"authway/src/server/internal/config"
	"authway/src/server/pkg/user"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service interface {
	// Authentication methods
	Authenticate(email, password string) (*user.User, error)
	// Additional auth methods can be added here
}

type service struct {
	db     *gorm.DB
	redis  *redis.Client
	config *config.Config
	logger *zap.Logger
}

func NewService(db *gorm.DB, redis *redis.Client, config *config.Config, logger *zap.Logger) Service {
	return &service{
		db:     db,
		redis:  redis,
		config: config,
		logger: logger,
	}
}

func (s *service) Authenticate(email, password string) (*user.User, error) {
	var foundUser user.User
	if err := s.db.Where("email = ?", email).First(&foundUser).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// This would typically use bcrypt to verify the password
	// For now, returning the user if found
	return &foundUser, nil
}
