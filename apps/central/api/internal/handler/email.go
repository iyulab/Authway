package handler

import (
	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/user"
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// EmailHandler handles email verification and password reset requests
// The answers to a verification or reset request do not depend on whether
// the address has an account, whether it is already verified, or whether the
// mail could be handed to the sender — any difference would tell a caller
// which addresses are registered.
const (
	verificationRequested  = "If the address has an unverified account, a verification link has been sent"
	passwordResetRequested = "If the address has an account, a password reset link has been sent"
)

type EmailHandler struct {
	emailRepo    *email.Repository
	emailSvc     email.EmailService
	userSvc      user.Service
	clientSvc    client.Service
	hydraClient  *hydra.Client
	validator    *validator.Validate
	logger       *zap.Logger
	auditService audit.Service
}

// NewEmailHandler creates a new email handler
func NewEmailHandler(
	emailRepo *email.Repository,
	emailSvc email.EmailService,
	userSvc user.Service,
	clientSvc client.Service,
	hydraClient *hydra.Client,
	validator *validator.Validate,
	logger *zap.Logger,
	auditService audit.Service,
) *EmailHandler {
	return &EmailHandler{
		emailRepo:    emailRepo,
		emailSvc:     emailSvc,
		userSvc:      userSvc,
		clientSvc:    clientSvc,
		hydraClient:  hydraClient,
		validator:    validator,
		logger:       logger,
		auditService: auditService,
	}
}

// resolveUserByEmail looks up the user for a self-service email flow
// (verification resend, password reset). When clientID is present, it scopes
// the lookup to that client's tenant (mirroring AuthHandler.Login's
// GetByClientID → GetByEmailAndTenant pattern) so a same-email user in a
// different tenant can never be matched. When clientID is empty — the caller
// has no OAuth context, e.g. a bookmarked link — it falls back to the
// unscoped lookup, same as before this tenant-scoping was added.
func (h *EmailHandler) resolveUserByEmail(clientID, emailAddr string) (*user.User, error) {
	if clientID == "" {
		return h.userSvc.GetByEmailUnscoped(emailAddr)
	}
	requestedClient, err := h.clientSvc.GetByClientID(clientID)
	if err != nil {
		h.logger.Warn("email flow: unknown client_id, falling back to unscoped lookup",
			zap.String("client_id", clientID), zap.Error(err))
		return h.userSvc.GetByEmailUnscoped(emailAddr)
	}
	return h.userSvc.GetByEmailAndTenant(requestedClient.TenantID, emailAddr)
}

// logUserAudit records an audit entry for a user self-service flow (anonymous
// request authenticated by a one-shot token). The resolved user is the actor.
func (h *EmailHandler) logUserAudit(c *fiber.Ctx, u *user.User, action audit.AuditAction, extra map[string]any) {
	if h.auditService == nil || u == nil {
		return
	}
	entry := audit.EntryFromFiber(c, u.TenantID, action, "user", u.ID.String())
	entry.ActorID = &u.ID
	entry.ActorEmail = u.Email
	entry.ActorType = "user"
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// SendVerificationEmail godoc
// @Summary Send email verification
// @Description Send verification email to user
// @Tags Email
// @Accept json
// @Produce json
// @Param request body email.SendVerificationRequest true "Send verification request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /api/v1/email/send-verification [post]
func (h *EmailHandler) SendVerificationEmail(c *fiber.Ctx) error {
	var req email.SendVerificationRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	if err := h.validator.Struct(req); err != nil {
		return apierror.Invalid(c, err)
	}

	// No mail for an account that is verified already or suspended; the answer
	// is the same either way, so it says nothing about the account.
	usr, err := h.resolveUserByEmail(req.ClientID, req.Email)
	if err != nil || usr.EmailVerified || !usr.Active {
		return c.JSON(fiber.Map{"message": verificationRequested})
	}

	// Delete old verifications for this user
	if err := h.emailRepo.DeleteVerificationsByUserID(usr.ID); err != nil {
		h.logger.Error("Failed to delete old verifications", zap.Error(err))
	}

	// Create new verification
	verification, err := h.emailRepo.CreateVerification(usr.ID)
	if err != nil {
		h.logger.Error("Failed to create verification", zap.Error(err))
		return apierror.Refuse(c, http.StatusInternalServerError, apierror.CodeForStatus(http.StatusInternalServerError), "Failed to create verification")
	}

	// Send verification email
	if err := h.emailSvc.SendVerificationEmail(usr.Email, verification.Token); err != nil {
		h.logger.Error("Failed to send verification email", zap.Error(err))
	}

	return c.JSON(fiber.Map{"message": verificationRequested})
}

// VerifyEmail godoc
// @Summary Verify email
// @Description Verify user email with token
// @Tags Email
// @Accept json
// @Produce json
// @Param token query string true "Verification token"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v1/email/verify [get]
func (h *EmailHandler) VerifyEmail(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return apierror.Refuse(c, http.StatusBadRequest, "invalid_request", "Token is required")
	}

	// Get verification by token
	verification, err := h.emailRepo.GetVerificationByToken(token)
	if err != nil {
		h.logger.Error("Verification token not found", zap.Error(err))
		return apierror.Refuse(c, http.StatusNotFound, "invalid_token", "Invalid or expired verification token")
	}

	// Check if expired
	if verification.IsExpired() {
		return apierror.Refuse(c, http.StatusBadRequest, "token_expired", "Verification token has expired")
	}

	// Mark verification as verified
	if err := h.emailRepo.MarkVerificationAsVerified(verification.ID); err != nil {
		h.logger.Error("Failed to mark verification as verified", zap.Error(err))
		return apierror.Refuse(c, http.StatusInternalServerError, apierror.CodeForStatus(http.StatusInternalServerError), "Failed to verify email")
	}

	// Update user email_verified status
	if err := h.userSvc.UpdateEmailVerified(verification.UserID, true); err != nil {
		h.logger.Error("Failed to update user email verified status", zap.Error(err))
		return apierror.Refuse(c, http.StatusInternalServerError, apierror.CodeForStatus(http.StatusInternalServerError), "Failed to update user status")
	}

	if verifiedUser, err := h.userSvc.GetByID(verification.UserID); err == nil {
		h.logUserAudit(c, verifiedUser, audit.ActionUserEmailVerified, nil)
	} else {
		h.logger.Warn("Failed to load user for email_verified audit", zap.Error(err))
	}

	return c.JSON(fiber.Map{
		"message": "Email verified successfully",
	})
}

// ForgotPassword godoc
// @Summary Request password reset
// @Description Send password reset link to email
// @Tags Email
// @Accept json
// @Produce json
// @Param request body email.ForgotPasswordRequest true "Forgot password request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v1/email/forgot-password [post]
func (h *EmailHandler) ForgotPassword(c *fiber.Ctx) error {
	var req email.ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	if err := h.validator.Struct(req); err != nil {
		return apierror.Invalid(c, err)
	}

	// A suspended account is sent nothing — it could not sign in with the new
	// password — and gets the same answer as an address with no account.
	usr, err := h.resolveUserByEmail(req.ClientID, req.Email)
	if err != nil || !usr.Active {
		return c.JSON(fiber.Map{"message": passwordResetRequested})
	}

	// Create password reset token
	reset, err := h.emailRepo.CreatePasswordReset(usr.ID)
	if err != nil {
		h.logger.Error("Failed to create password reset", zap.Error(err))
		return apierror.Refuse(c, http.StatusInternalServerError, apierror.CodeForStatus(http.StatusInternalServerError), "Failed to create password reset")
	}

	// Send reset email
	if err := h.emailSvc.SendPasswordResetEmail(usr.Email, reset.Token); err != nil {
		h.logger.Error("Failed to send reset email", zap.Error(err))
	}

	return c.JSON(fiber.Map{"message": passwordResetRequested})
}

// VerifyResetToken godoc
// @Summary Verify reset token
// @Description Verify if password reset token is valid
// @Tags Email
// @Accept json
// @Produce json
// @Param token query string true "Reset token"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v1/email/verify-reset-token [get]
func (h *EmailHandler) VerifyResetToken(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return apierror.Refuse(c, http.StatusBadRequest, "invalid_request", "Token is required")
	}

	// Get reset by token
	reset, err := h.emailRepo.GetPasswordResetByToken(token)
	if err != nil {
		return apierror.RefuseWith(c, http.StatusBadRequest, "invalid_token", "Invalid or expired reset token", nil)
	}

	// Check if valid (not used and not expired)
	if !reset.IsValid() {
		return apierror.RefuseWith(c, http.StatusBadRequest, "token_expired", "Reset token is invalid or has expired", nil)
	}

	// The length the new password has to have is the tenant's of the account
	// being reset. An account that is gone leaves the reset to fail when it
	// is submitted; the screen is told what every tenant asks.
	least := user.MinPasswordLength
	if account, err := h.userSvc.GetByID(reset.UserID); err == nil {
		least = h.userSvc.LeastPasswordLength(account.TenantID)
	}

	return c.JSON(fiber.Map{
		"valid":               true,
		"message":             "Reset token is valid",
		"password_min_length": least,
	})
}

// ResetPassword godoc
// @Summary Reset password
// @Description Reset password with valid token
// @Tags Email
// @Accept json
// @Produce json
// @Param request body email.ResetPasswordRequest true "Reset password request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v1/email/reset-password [post]
func (h *EmailHandler) ResetPassword(c *fiber.Ctx) error {
	var req email.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	if err := h.validator.Struct(req); err != nil {
		return apierror.Invalid(c, err)
	}

	// Get reset by token
	reset, err := h.emailRepo.GetPasswordResetByToken(req.Token)
	if err != nil {
		return apierror.Refuse(c, http.StatusBadRequest, "invalid_token", "Invalid or expired reset token")
	}

	// Check if valid
	if !reset.IsValid() {
		return apierror.Refuse(c, http.StatusBadRequest, "token_expired", "Reset token is invalid or has expired")
	}

	resetUser, resetUserErr := h.userSvc.GetByID(reset.UserID)
	if resetUserErr != nil {
		h.logger.Warn("Failed to load user for password_reset audit", zap.Error(resetUserErr))
	}

	// Update user password
	if err := h.userSvc.UpdatePassword(reset.UserID, req.NewPassword); err != nil {
		var refused *user.PasswordError
		if errors.As(err, &refused) {
			return apierror.RefuseWith(c, http.StatusBadRequest, refused.Code(), refused.Error(), fiber.Map{"min_length": refused.MinLength})
		}
		h.logger.Error("Failed to update password", zap.Error(err))
		return apierror.Refuse(c, http.StatusInternalServerError, apierror.CodeForStatus(http.StatusInternalServerError), "Failed to update password")
	}

	h.logUserAudit(c, resetUser, audit.ActionUserPasswordReset, nil)

	// The reset link reached the account's address: using it proves the
	// address as a verification mail would.
	if resetUser != nil && !resetUser.EmailVerified {
		if err := h.userSvc.UpdateEmailVerified(reset.UserID, true); err != nil {
			h.logger.Error("Failed to mark the address verified after a password reset", zap.Error(err))
		}
	}

	// Mark reset as used
	if err := h.emailRepo.MarkPasswordResetAsUsed(reset.ID); err != nil {
		h.logger.Error("Failed to mark reset as used", zap.Error(err))
	}

	// Invalidate all user sessions for security
	if err := h.hydraClient.RevokeUserSessions(reset.UserID.String()); err != nil {
		h.logger.Error("Failed to revoke user sessions after password reset",
			zap.String("user_id", reset.UserID.String()),
			zap.Error(err))
		// Don't fail the request even if session revocation fails
		// The password has already been reset successfully
	} else {
		h.logger.Info("Successfully revoked all user sessions after password reset",
			zap.String("user_id", reset.UserID.String()))
	}

	return c.JSON(fiber.Map{
		"message": "Password reset successfully",
	})
}

// RegisterRoutes registers email routes
func (h *EmailHandler) RegisterRoutes(router fiber.Router) {
	email := router.Group("/email")
	{
		email.Post("/send-verification", h.SendVerificationEmail)
		email.Get("/verify", h.VerifyEmail)
		email.Post("/forgot-password", h.ForgotPassword)
		email.Get("/verify-reset-token", h.VerifyResetToken)
		email.Post("/reset-password", h.ResetPassword)
	}
}
