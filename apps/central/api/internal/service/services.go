package service

import (
	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Services holds all application services
type Services struct {
	UserService   user.Service
	ClientService client.Service
}

// NewUserService creates a new user service
func NewUserService(db *gorm.DB, logger *zap.Logger) user.Service {
	return user.NewService(db, logger)
}

// NewClientService creates a new client service
func NewClientService(db *gorm.DB, logger *zap.Logger, hydraClient *hydra.Client) client.Service {
	return client.NewService(db, logger, hydraClient)
}
