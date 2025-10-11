package handler

import (
	"authway/src/server/internal/hydra"
	"authway/src/server/pkg/email"
	"authway/src/server/pkg/user"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// EmailHandler handles email verification and password reset requests
type EmailHandler struct {
	emailRepo   *email.Repository
	emailSvc    *email.Service
	userSvc     user.Service
	hydraClient *hydra.Client
	validator   *validator.Validate
	logger      *zap.Logger
}

// NewEmailHandler creates a new email handler
func NewEmailHandler(
	emailRepo *email.Repository,
	emailSvc *email.Service,
	userSvc user.Service,
	hydraClient *hydra.Client,
	validator *validator.Validate,
	logger *zap.Logger,
) *EmailHandler {
	return &EmailHandler{
		emailRepo:   emailRepo,
		emailSvc:    emailSvc,
		userSvc:     userSvc,
		hydraClient: hydraClient,
		validator:   validator,
		logger:      logger,
	}
}

// SendVerificationEmail godoc
// @Summary Send email verification
// @Description Send verification email to user
// @Tags Email
// @Accept json
// @Produce json
// @Param request body email.SendVerificationRequest true "Send verification request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/email/send-verification [post]
func (h *EmailHandler) SendVerificationEmail(c *fiber.Ctx) error {
	var req email.SendVerificationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := h.validator.Struct(req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Find user by email
	usr, err := h.userSvc.GetByEmail(req.Email)
	if err != nil {
		// Don't reveal if email exists or not (security)
		return c.JSON(fiber.Map{
			"message": "If the email exists, a verification link has been sent",
		})
	}

	// Check if already verified
	if usr.EmailVerified {
		return c.JSON(fiber.Map{
			"message": "Email already verified",
		})
	}

	// Delete old verifications for this user
	if err := h.emailRepo.DeleteVerificationsByUserID(usr.ID); err != nil {
		h.logger.Error("Failed to delete old verifications", zap.Error(err))
	}

	// Create new verification
	verification, err := h.emailRepo.CreateVerification(usr.ID)
	if err != nil {
		h.logger.Error("Failed to create verification", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create verification",
		})
	}

	// Send verification email
	if err := h.emailSvc.SendVerificationEmail(usr.Email, verification.Token); err != nil {
		h.logger.Error("Failed to send verification email", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to send verification email",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Verification email sent successfully",
	})
}

// VerifyEmail godoc
// @Summary Verify email
// @Description Verify user email with token
// @Tags Email
// @Accept json
// @Produce json
// @Param token query string true "Verification token"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/email/verify [get]
func (h *EmailHandler) VerifyEmail(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Token is required",
		})
	}

	// Get verification by token
	verification, err := h.emailRepo.GetVerificationByToken(token)
	if err != nil {
		h.logger.Error("Verification token not found", zap.Error(err))
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Invalid or expired verification token",
		})
	}

	// Check if expired
	if verification.IsExpired() {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Verification token has expired",
		})
	}

	// Mark verification as verified
	if err := h.emailRepo.MarkVerificationAsVerified(verification.ID); err != nil {
		h.logger.Error("Failed to mark verification as verified", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to verify email",
		})
	}

	// Update user email_verified status
	if err := h.userSvc.UpdateEmailVerified(verification.UserID, true); err != nil {
		h.logger.Error("Failed to update user email verified status", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user status",
		})
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
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/email/forgot-password [post]
func (h *EmailHandler) ForgotPassword(c *fiber.Ctx) error {
	var req email.ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := h.validator.Struct(req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Find user by email
	usr, err := h.userSvc.GetByEmail(req.Email)
	if err != nil {
		// Don't reveal if email exists or not (security)
		return c.JSON(fiber.Map{
			"message": "If the email exists, a password reset link has been sent",
		})
	}

	// Create password reset token
	reset, err := h.emailRepo.CreatePasswordReset(usr.ID)
	if err != nil {
		h.logger.Error("Failed to create password reset", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create password reset",
		})
	}

	// Send reset email
	if err := h.emailSvc.SendPasswordResetEmail(usr.Email, reset.Token); err != nil {
		h.logger.Error("Failed to send reset email", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to send reset email",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Password reset link sent successfully",
	})
}

// VerifyResetToken godoc
// @Summary Verify reset token
// @Description Verify if password reset token is valid
// @Tags Email
// @Accept json
// @Produce json
// @Param token query string true "Reset token"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/email/verify-reset-token [get]
func (h *EmailHandler) VerifyResetToken(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Token is required",
		})
	}

	// Get reset by token
	reset, err := h.emailRepo.GetPasswordResetByToken(token)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or expired reset token",
			"valid": false,
		})
	}

	// Check if valid (not used and not expired)
	if !reset.IsValid() {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Reset token is invalid or has expired",
			"valid": false,
		})
	}

	return c.JSON(fiber.Map{
		"valid":   true,
		"message": "Reset token is valid",
	})
}

// ResetPassword godoc
// @Summary Reset password
// @Description Reset password with valid token
// @Tags Email
// @Accept json
// @Produce json
// @Param request body email.ResetPasswordRequest true "Reset password request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/email/reset-password [post]
func (h *EmailHandler) ResetPassword(c *fiber.Ctx) error {
	var req email.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := h.validator.Struct(req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Get reset by token
	reset, err := h.emailRepo.GetPasswordResetByToken(req.Token)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or expired reset token",
		})
	}

	// Check if valid
	if !reset.IsValid() {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Reset token is invalid or has expired",
		})
	}

	// Update user password
	if err := h.userSvc.UpdatePassword(reset.UserID, req.NewPassword); err != nil {
		h.logger.Error("Failed to update password", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update password",
		})
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
