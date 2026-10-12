package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/mfa"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/user"
)

type MFAHandler struct {
	mfaService   mfa.Service
	userService  user.Service
	logger       *zap.Logger
	auditService audit.Service
	sessions     SessionEnder
}

// SessionEnder ends a user's sessions at the authorization server.
type SessionEnder interface {
	// RevokeOtherSessions ends every session of subject but keep (all of
	// them when keep is empty) and their tokens.
	RevokeOtherSessions(subject, keep string) error
}

func NewMFAHandler(mfaService mfa.Service, userService user.Service, logger *zap.Logger, auditService audit.Service, sessions SessionEnder) *MFAHandler {
	return &MFAHandler{
		sessions:     sessions,
		mfaService:   mfaService,
		userService:  userService,
		logger:       logger,
		auditService: auditService,
	}
}

type VerifyMFARequest struct {
	Code string `json:"code" validate:"required,len=6"`
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
	if errors.Is(err, mfa.ErrAlreadyEnabled) {
		return apierror.Refuse(c, fiber.StatusConflict, "mfa_already_enabled", err.Error())
	}
	if err != nil {
		h.logger.Error("MFA setup failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to set up MFA")})
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to verify MFA setup")})
	}
	// A session opened before now was opened on one factor, and so were the
	// tokens issued in it: whoever holds one would keep the account past the
	// second factor. They end here. This session stays — its user has just
	// given a code, and the answer carries the recovery codes they have to
	// write down. A token that does not say which session it came from
	// cannot be told apart, so then every session ends, this one included.
	keep, _ := c.Locals("session_id").(string)
	ended := "others"
	if keep == "" {
		ended = "all"
	}
	if err := h.sessions.RevokeOtherSessions(userID.String(), keep); err != nil && !errors.Is(err, hydra.ErrApplicationsNotNotified) {
		// The factor is on either way; say what did not happen.
		h.logger.Error("Second factor enabled, but the user's earlier sessions could not be ended",
			zap.Error(err), zap.String("user_id", userID.String()))
		ended = "none"
	}
	h.logMFASuccess(c, userID, audit.ActionUserMFAEnabled, audit.SeverityWarning, map[string]any{"sessions_ended": ended})
	return c.JSON(fiber.Map{
		"recovery_codes": resp.RecoveryCodes,
		"message":        resp.Message,
		"sessions_ended": ended,
	})
}

// DisableMFA disables MFA for the user
// DELETE /api/v1/users/mfa
func (h *MFAHandler) DisableMFA(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	proof := factorProof(c)
	err := h.mfaService.Disable(userID, proof)
	if errors.Is(err, mfa.ErrProofRequired) {
		return h.refuseWithoutProof(c, userID, "disable")
	}
	if err != nil {
		h.logger.Error("MFA disable failed", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to disable MFA")})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": apierror.Message(err, "failed to get MFA status")})
	}
	return c.JSON(status)
}

// RegenerateRecoveryCodes generates new recovery codes
// POST /api/v1/users/mfa/recovery/regenerate
func (h *MFAHandler) RegenerateRecoveryCodes(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	resp, err := h.mfaService.RegenerateRecoveryCodes(userID, factorProof(c))
	if errors.Is(err, mfa.ErrProofRequired) {
		return h.refuseWithoutProof(c, userID, "regenerate_recovery_codes")
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to regenerate recovery codes")})
	}
	return c.JSON(resp)
}

// factorProof reads the proof of the second factor that comes with a request
// to change it: {"code": "..."} — a current authenticator code or an unused
// recovery code.
func factorProof(c *fiber.Ctx) string {
	var body struct {
		Code string `json:"code"`
	}
	_ = c.BodyParser(&body)
	return strings.TrimSpace(body.Code)
}

// refuseWithoutProof answers a change to the second factor that came without
// valid proof. It counts against the caller's address like a failed sign-in:
// the caller already holds the user's token and is guessing the other factor.
func (h *MFAHandler) refuseWithoutProof(c *fiber.Ctx, userID uuid.UUID, phase string) error {
	middleware.IncrementRateLimitOnFailure(c)
	h.logMFAFailure(c, userID, audit.ActionUserMFAFailed, mfa.ErrProofRequired.Error(), map[string]any{"phase": phase})
	return apierror.Refuse(c, fiber.StatusForbidden, "mfa_code_required", mfa.ErrProofRequired.Error())
}
