package service

import (
	// "authway/src/server/internal/config"  // Currently unused
	// "authway/src/server/pkg/auth"         // Package not yet implemented
	"authway/src/server/pkg/client"
	// "authway/src/server/pkg/token"       // Package not yet implemented
	"authway/src/server/pkg/user"
	// "github.com/redis/go-redis/v9"       // Currently unused
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Services holds all application services
type Services struct {
	UserService   user.Service
	ClientService client.Service
	// TokenService  token.Service  // Package not yet implemented
	// AuthService   auth.Service   // Package not yet implemented
}

// NewUserService creates a new user service
func NewUserService(db *gorm.DB, logger *zap.Logger) user.Service {
	return user.NewService(db, logger)
}

// NewClientService creates a new client service
func NewClientService(db *gorm.DB, logger *zap.Logger) client.Service {
	return client.NewService(db, logger)
}

// NewTokenService creates a new token service - Package not yet implemented
// func NewTokenService(redis *redis.Client, jwtConfig config.JWTConfig, logger *zap.Logger) token.Service {
//	return token.NewService(redis, jwtConfig, logger)
// }

// NewAuthService creates a new auth service - Package not yet implemented
// func NewAuthService(db *gorm.DB, redis *redis.Client, cfg *config.Config, logger *zap.Logger) auth.Service {
//	return auth.NewService(db, redis, cfg, logger)
// }
