package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/mfa"
)

type MFAHandler struct {
	mfaService mfa.Service
	logger     *zap.Logger
}

func NewMFAHandler(mfaService mfa.Service, logger *zap.Logger) *MFAHandler {
	return &MFAHandler{mfaService: mfaService, logger: logger}
}

type VerifyMFARequest struct {
	Code string `json:"code" validate:"required,len=6"`
}

type VerifyRecoveryCodeRequest struct {
	Code string `json:"code" validate:"required"`
}

// SetupMFA initiates TOTP setup
// POST /api/v1/users/mfa/setup
func (h *MFAHandler) SetupMFA(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}
	resp, err := h.mfaService.SetupTOTP(userID)
	if err != nil {
		h.logger.Error("MFA setup failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}
// VerifyMFA verifies TOTP code and enables MFA
// POST /api/v1/users/mfa/verify
func (h *MFAHandler) VerifyMFA(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}
	var req VerifyMFARequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	resp, err := h.mfaService.VerifyAndEnable(userID, req.Code)
	if err != nil {
		h.logger.Warn("MFA verification failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// DisableMFA disables MFA for the user
// DELETE /api/v1/users/mfa
func (h *MFAHandler) DisableMFA(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}
	if err := h.mfaService.Disable(userID); err != nil {
		h.logger.Error("MFA disable failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "MFA disabled successfully"})
}
// GetMFAStatus returns MFA status
// GET /api/v1/users/mfa/status
func (h *MFAHandler) GetMFAStatus(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}
	status, err := h.mfaService.GetStatus(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(status)
}

// VerifyRecoveryCode verifies and consumes a recovery code
// POST /api/v1/users/mfa/recovery
func (h *MFAHandler) VerifyRecoveryCode(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}
	var req VerifyRecoveryCodeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	valid, err := h.mfaService.VerifyRecoveryCode(userID, req.Code)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if !valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid recovery code"})
	}
	return c.JSON(fiber.Map{"message": "Recovery code verified", "valid": true})
}

// RegenerateRecoveryCodes generates new recovery codes
// POST /api/v1/users/mfa/recovery/regenerate
func (h *MFAHandler) RegenerateRecoveryCodes(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}
	resp, err := h.mfaService.RegenerateRecoveryCodes(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}
