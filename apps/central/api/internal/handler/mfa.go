package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/mfa"
	"authway/apps/central/api/pkg/user"
)

type MFAHandler struct {
	mfaService   mfa.Service
	userService  user.Service
	logger       *zap.Logger
	auditService audit.Service
}

func NewMFAHandler(mfaService mfa.Service, userService user.Service, logger *zap.Logger, auditService audit.Service) *MFAHandler {
	return &MFAHandler{
		mfaService:   mfaService,
		userService:  userService,
		logger:       logger,
		auditService: auditService,
	}
}

type VerifyMFARequest struct {
	Code string `json:"code" validate:"required,len=6"`
}

type VerifyRecoveryCodeRequest struct {
	Code string `json:"code" validate:"required"`
}

// tenantForUser resolves a user's tenant for audit entries. Unknown users fall
// back to uuid.Nil so the audit write never fails on its own lookup — the row
// is still queryable by actor_id / resource_id.
func (h *MFAHandler) tenantForUser(userID uuid.UUID) uuid.UUID {
	if h.userService == nil {
		return uuid.Nil
	}
	u, err := h.userService.GetByID(userID)
	if err != nil || u == nil {
		return uuid.Nil
	}
	return u.TenantID
}

// logMFASuccess emits a best-effort (async) audit entry for a successful MFA
// event. Buffer drops are acceptable here — failure paths use logMFAFailure.
func (h *MFAHandler) logMFASuccess(c *fiber.Ctx, userID uuid.UUID, action audit.AuditAction, severity audit.AuditSeverity, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, h.tenantForUser(userID), action, "user_mfa", userID.String())
	entry.Severity = severity
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// logMFAFailure emits a sync audit entry for a failed MFA verification. Sync
// (not Async) because security-critical failures must not be dropped on buffer
// overflow — this is the write path a lockout investigation relies on.
func (h *MFAHandler) logMFAFailure(c *fiber.Ctx, userID uuid.UUID, action audit.AuditAction, errMsg string, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, h.tenantForUser(userID), action, "user_mfa", userID.String())
	entry.Severity = audit.SeverityWarning
	entry.Success = false
	entry.ErrorMsg = errMsg
	for k, v := range extra {
		entry.Details[k] = v
	}
	if err := h.auditService.Log(c.UserContext(), entry); err != nil {
		h.logger.Error("Failed to write MFA failure audit log", zap.Error(err), zap.String("user_id", userID.String()))
	}
}

// SetupMFA initiates TOTP setup
// POST /api/v1/users/mfa/setup
func (h *MFAHandler) SetupMFA(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	resp, err := h.mfaService.SetupTOTP(userID)
	if err != nil {
		h.logger.Error("MFA setup failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Setup alone does not enable MFA — the audit event fires on VerifyMFA
	// success. Logging setup separately would just double every enablement.
	return c.JSON(resp)
}

// VerifyMFA verifies TOTP code and enables MFA
// POST /api/v1/users/mfa/verify
func (h *MFAHandler) VerifyMFA(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req VerifyMFARequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	resp, err := h.mfaService.VerifyAndEnable(userID, req.Code)
	if err != nil {
		h.logger.Warn("MFA verification failed", zap.Error(err), zap.String("user_id", userID.String()))
		h.logMFAFailure(c, userID, audit.ActionUserMFAFailed, err.Error(), map[string]any{
			"phase": "enable",
		})
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	h.logMFASuccess(c, userID, audit.ActionUserMFAEnabled, audit.SeverityWarning, nil)
	return c.JSON(resp)
}

// DisableMFA disables MFA for the user
// DELETE /api/v1/users/mfa
func (h *MFAHandler) DisableMFA(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if err := h.mfaService.Disable(userID); err != nil {
		h.logger.Error("MFA disable failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	h.logMFASuccess(c, userID, audit.ActionUserMFADisabled, audit.SeverityWarning, nil)
	return c.JSON(fiber.Map{"message": "MFA disabled successfully"})
}

// GetMFAStatus returns MFA status
// GET /api/v1/users/mfa/status
func (h *MFAHandler) GetMFAStatus(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
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
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req VerifyRecoveryCodeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	valid, err := h.mfaService.VerifyRecoveryCode(userID, req.Code)
	if err != nil {
		h.logMFAFailure(c, userID, audit.ActionUserMFAFailed, err.Error(), map[string]any{
			"phase": "recovery_code",
		})
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if !valid {
		h.logMFAFailure(c, userID, audit.ActionUserMFAFailed, "invalid recovery code", map[string]any{
			"phase": "recovery_code",
		})
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid recovery code"})
	}
	h.logMFASuccess(c, userID, audit.ActionUserMFAVerified, audit.SeverityInfo, map[string]any{
		"phase": "recovery_code",
	})
	return c.JSON(fiber.Map{"message": "Recovery code verified", "valid": true})
}

// RegenerateRecoveryCodes generates new recovery codes
// POST /api/v1/users/mfa/recovery/regenerate
func (h *MFAHandler) RegenerateRecoveryCodes(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	resp, err := h.mfaService.RegenerateRecoveryCodes(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}
