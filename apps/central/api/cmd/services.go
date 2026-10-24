package main

import (
	"time"

	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/email"
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
	AuditService        audit.Service
	AuditHandler        *audit.Handler
	WebhookService      webhook.Service
	WebhookHandler      *webhook.Handler
	InvitationService   invitation.Service
	InvitationHandler   *invitation.Handler
	PasswordlessService passwordless.Service
}

// InitNewFeatureServices initializes all new feature services
func InitNewFeatureServices(
	db *gorm.DB,
	logger *zap.Logger,
	userService user.Service,
	tenantService *tenant.Service,
	emailService email.EmailService,
	auditService audit.Service,
	frontendURL string,
	webhookAllowPrivateTargets bool,
	atRest crypto.Cipher,
) *NewFeatureServices {
	auditHandler := audit.NewHandler(auditService, logger, tenant.DefaultAuditRetentionDays)

	// Webhook Service. Every event a webhook can subscribe to is something
	// the audit log records, so webhooks listen to what is recorded there.
	webhookService := webhook.NewService(db, logger, webhook.AllowPrivateTargets(webhookAllowPrivateTargets))
	webhookHandler := webhook.NewHandler(webhookService, logger, auditService)
	auditService.Subscribe(webhook.FromAudit(webhookService, logger))

	// Invitation Service
	invitationEmailAdapter := &invitationEmailAdapterImpl{
		emailService: emailService,
	}
	invitationService := invitation.NewService(db, userService, tenantService, invitationEmailAdapter, logger, frontendURL)
	invitationHandler := invitation.NewHandler(invitationService, logger)

	// Passwordless Service
	passwordlessEmailAdapter := &passwordlessEmailAdapterImpl{
		emailService: emailService,
	}
	// invitationService doubles as the passwordless invitation gate — magic-link
	// provisioning is allowed only for an invited address (invitation-only).
	passwordlessService := passwordless.NewService(db, userService, invitationService, passwordlessEmailAdapter, logger, frontendURL, atRest, tenantService.Name)

	return &NewFeatureServices{
		AuditService:        auditService,
		AuditHandler:        auditHandler,
		WebhookService:      webhookService,
		WebhookHandler:      webhookHandler,
		InvitationService:   invitationService,
		InvitationHandler:   invitationHandler,
		PasswordlessService: passwordlessService,
	}
}

// RegisterRoutes registers all new feature routes
func (s *NewFeatureServices) RegisterRoutes(v1 fiber.Router, jwtAuth, adminAuth, auditReadAuth fiber.Handler) {

	// Invitation routes (mixed public/protected)
	s.InvitationHandler.RegisterRoutes(v1, jwtAuth, adminAuth)

	// Webhook management routes (admin only)
	s.WebhookHandler.RegisterRoutes(v1, jwtAuth, adminAuth)

	// Audit log routes: the admin API, or a tenant's service credential
	// granted admin.audit:read.
	s.AuditHandler.RegisterRoutes(v1, adminAuth, auditReadAuth)
}

// How long records that name people are kept once they have served:
// webhook delivery records (with the receivers' answers) and invitations
// that were accepted, revoked or expired. The audit log's is the tenant's
// setting (tenant.DefaultAuditRetentionDays when it has none).
const (
	deliveryRetention         = 30 * 24 * time.Hour
	closedInvitationRetention = 90 * 24 * time.Hour
)

// purgeExpiredRecords removes what has outlived its retention.
func (s *NewFeatureServices) purgeExpiredRecords(logger *zap.Logger) {
	for _, job := range []struct {
		what  string
		purge func() (int64, error)
	}{
		{"audit entries", func() (int64, error) { return s.AuditService.PurgeExpired(tenant.DefaultAuditRetentionDays) }},
		{"webhook deliveries", func() (int64, error) { return s.WebhookService.PurgeDeliveries(deliveryRetention) }},
		{"closed invitations", func() (int64, error) { return s.InvitationService.PurgeClosed(closedInvitationRetention) }},
	} {
		if count, err := job.purge(); err != nil {
			logger.Error("Failed to remove expired records", zap.String("records", job.what), zap.Error(err))
		} else if count > 0 {
			logger.Info("Removed expired records", zap.String("records", job.what), zap.Int64("count", count))
		}
	}
}

// StartBackgroundCleanupTasks starts background cleanup goroutines
func (s *NewFeatureServices) StartBackgroundCleanupTasks(logger *zap.Logger) {
	// Records past their retention go once at start — an instance that
	// scales to zero may never see a day's tick — and daily after.
	go func() {
		s.purgeExpiredRecords(logger)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.purgeExpiredRecords(logger)
		}
	}()

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

// invitationEmailAdapterImpl implements invitation.EmailSender by delegating
// to the central email.EmailService (Sendway in prod, SMTP in dev).
type invitationEmailAdapterImpl struct {
	emailService email.EmailService
}

func (a *invitationEmailAdapterImpl) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	return a.emailService.SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL)
}

// passwordlessEmailAdapterImpl implements passwordless.EmailSender by delegating
// to the central email.EmailService.
type passwordlessEmailAdapterImpl struct {
	emailService email.EmailService
}

func (a *passwordlessEmailAdapterImpl) SendMagicLinkEmail(toEmail, organization, linkURL, code string, isNewUser bool) error {
	return a.emailService.SendMagicLinkEmail(toEmail, organization, linkURL, code, isNewUser)
}
