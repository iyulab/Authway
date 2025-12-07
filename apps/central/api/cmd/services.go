package main

import (
	"fmt"
	"time"

	"authway/apps/central/api/pkg/accountlink"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/impersonation"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/passwordless"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"authway/apps/central/api/pkg/webhook"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NewFeatureServices holds all new feature services
type NewFeatureServices struct {
	AuditService         audit.Service
	AuditHandler         *audit.Handler
	WebhookService       webhook.Service
	WebhookHandler       *webhook.Handler
	AccountLinkService   accountlink.Service
	AccountLinkHandler   *accountlink.Handler
	InvitationService    invitation.Service
	InvitationHandler    *invitation.Handler
	PasswordlessService  passwordless.Service
	PasswordlessHandler  *passwordless.Handler
	ImpersonationService impersonation.Service
	ImpersonationHandler *impersonation.Handler
}

// InitNewFeatureServices initializes all new feature services
func InitNewFeatureServices(
	db *gorm.DB,
	logger *zap.Logger,
	userService user.Service,
	tenantService *tenant.Service,
	frontendURL string,
) *NewFeatureServices {
	// Audit Service
	auditService := audit.NewService(db, logger)
	auditHandler := audit.NewHandler(auditService, logger)

	// Webhook Service
	webhookService := webhook.NewService(db, logger)
	webhookHandler := webhook.NewHandler(webhookService, logger)

	// Account Linking Service
	accountLinkService := accountlink.NewService(db, userService, logger)
	accountLinkHandler := accountlink.NewHandler(accountLinkService, logger)

	// Invitation Service
	invitationEmailAdapter := &invitationEmailAdapterImpl{
		frontendURL: frontendURL,
	}
	invitationService := invitation.NewService(db, userService, tenantService, invitationEmailAdapter, logger, frontendURL)
	invitationHandler := invitation.NewHandler(invitationService, logger)

	// Passwordless Service
	passwordlessEmailAdapter := &passwordlessEmailAdapterImpl{
		frontendURL: frontendURL,
	}
	passwordlessService := passwordless.NewService(db, userService, passwordlessEmailAdapter, logger, frontendURL)
	passwordlessHandler := passwordless.NewHandler(passwordlessService, logger)

	// Impersonation Service
	impersonationService := impersonation.NewService(db, userService, auditService, logger)
	impersonationHandler := impersonation.NewHandler(impersonationService, logger)

	return &NewFeatureServices{
		AuditService:         auditService,
		AuditHandler:         auditHandler,
		WebhookService:       webhookService,
		WebhookHandler:       webhookHandler,
		AccountLinkService:   accountLinkService,
		AccountLinkHandler:   accountLinkHandler,
		InvitationService:    invitationService,
		InvitationHandler:    invitationHandler,
		PasswordlessService:  passwordlessService,
		PasswordlessHandler:  passwordlessHandler,
		ImpersonationService: impersonationService,
		ImpersonationHandler: impersonationHandler,
	}
}

// RegisterRoutes registers all new feature routes
func (s *NewFeatureServices) RegisterRoutes(v1 fiber.Router, jwtAuth, adminAuth fiber.Handler) {
	// Passwordless authentication routes (public endpoints)
	s.PasswordlessHandler.RegisterRoutes(v1)

	// Invitation routes (mixed public/protected)
	s.InvitationHandler.RegisterRoutes(v1, jwtAuth, adminAuth)

	// Account linking routes (authenticated)
	s.AccountLinkHandler.RegisterRoutes(v1, jwtAuth)

	// Webhook management routes (admin only)
	s.WebhookHandler.RegisterRoutes(v1, jwtAuth, adminAuth)

	// Audit log routes (admin only)
	s.AuditHandler.RegisterRoutes(v1, jwtAuth, adminAuth)

	// Impersonation routes (admin only)
	s.ImpersonationHandler.RegisterRoutes(v1, jwtAuth, adminAuth)
}

// StartBackgroundCleanupTasks starts background cleanup goroutines
func (s *NewFeatureServices) StartBackgroundCleanupTasks(logger *zap.Logger) {
	// Cleanup expired invitations periodically
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if count, err := s.InvitationService.CleanupExpired(); err != nil {
				logger.Error("Failed to cleanup expired invitations", zap.Error(err))
			} else if count > 0 {
				logger.Info("Cleaned up expired invitations", zap.Int64("count", count))
			}
		}
	}()

	// Cleanup expired magic links periodically
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if count, err := s.PasswordlessService.CleanupExpired(); err != nil {
				logger.Error("Failed to cleanup expired magic links", zap.Error(err))
			} else if count > 0 {
				logger.Info("Cleaned up expired magic links", zap.Int64("count", count))
			}
		}
	}()
}

// ======================================
// Email Adapter Implementations
// ======================================

// invitationEmailAdapterImpl implements invitation.EmailSender
type invitationEmailAdapterImpl struct {
	frontendURL string
}

func (a *invitationEmailAdapterImpl) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	// TODO: Integrate with actual email service when available
	// For now, log the invitation details
	fmt.Printf("INVITATION EMAIL: To=%s, Inviter=%s, Tenant=%s, URL=%s\n", toEmail, inviterName, tenantName, inviteURL)
	return nil
}

// passwordlessEmailAdapterImpl implements passwordless.EmailSender
type passwordlessEmailAdapterImpl struct {
	frontendURL string
}

func (a *passwordlessEmailAdapterImpl) SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error {
	// TODO: Integrate with actual email service when available
	// For now, log the magic link details
	userType := "existing"
	if isNewUser {
		userType = "new"
	}
	fmt.Printf("MAGIC LINK EMAIL: To=%s, URL=%s, UserType=%s\n", toEmail, linkURL, userType)
	return nil
}
