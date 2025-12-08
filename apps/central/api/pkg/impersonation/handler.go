package impersonation

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles impersonation HTTP requests
type Handler struct {
	service Service
	logger  *zap.Logger
}

// NewHandler creates a new impersonation handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// StartImpersonation starts an impersonation session
// POST /api/v1/admin/impersonate
func (h *Handler) StartImpersonation(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized - tenant_id required"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	// For Admin Console requests, use a system UUID for admin
	var adminID uuid.UUID
	isAdminConsole := c.Locals("is_admin_console")
	adminIDStr := c.Locals("user_id")

	if adminIDStr != nil {
		adminID, err = uuid.Parse(adminIDStr.(string))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid admin ID"})
		}
	} else if isAdminConsole != nil && isAdminConsole.(bool) {
		// Admin Console request - use a deterministic system UUID for admin
		adminID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	} else {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized - admin_id required"})
	}

	var req StartImpersonationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.TargetUserID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "target_user_id is required"})
	}

	if len(req.Reason) < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "reason must be at least 10 characters"})
	}

	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	response, err := h.service.StartImpersonation(tenantID, adminID, &req, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("Failed to start impersonation",
			zap.Error(err),
			zap.String("admin_id", adminID.String()),
			zap.String("target_user_id", req.TargetUserID.String()),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Impersonation started",
		zap.String("admin_id", adminID.String()),
		zap.String("target_user_id", req.TargetUserID.String()),
		zap.String("reason", req.Reason),
	)

	return c.JSON(fiber.Map{
		"message":       "impersonation session started",
		"token":         response.Token,
		"expires_at":    response.ExpiresAt,
		"target_user":   response.TargetUser,
	})
}

// ValidateImpersonationToken validates an impersonation token
// POST /api/v1/admin/impersonate/validate
func (h *Handler) ValidateImpersonationToken(c *fiber.Ctx) error {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if body.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	session, err := h.service.ValidateToken(body.Token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"valid":      true,
		"session_id": session.ID.String(),
		"admin": fiber.Map{
			"id":    session.AdminID.String(),
			"email": session.AdminEmail,
		},
		"target_user": fiber.Map{
			"id":    session.TargetUserID.String(),
			"email": session.TargetUserEmail,
		},
		"expires_at": session.ExpiresAt,
	})
}

// EndImpersonation ends an impersonation session
// POST /api/v1/admin/impersonate/:sessionId/end
func (h *Handler) EndImpersonation(c *fiber.Ctx) error {
	sessionIDStr := c.Params("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session ID"})
	}

	if err := h.service.EndImpersonation(sessionID); err != nil {
		h.logger.Warn("Failed to end impersonation", zap.Error(err), zap.String("session_id", sessionIDStr))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Impersonation ended", zap.String("session_id", sessionIDStr))
	return c.JSON(fiber.Map{"message": "impersonation session ended"})
}

// GetActiveSessions gets all active impersonation sessions
// GET /api/v1/admin/impersonate/sessions
func (h *Handler) GetActiveSessions(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	sessions, err := h.service.GetActiveSessions(tenantID)
	if err != nil {
		h.logger.Error("Failed to get active sessions", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get sessions"})
	}

	return c.JSON(fiber.Map{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// GetSessionHistory gets impersonation session history
// GET /api/v1/admin/impersonate/history
func (h *Handler) GetSessionHistory(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	sessions, err := h.service.GetSessionHistory(tenantID, limit)
	if err != nil {
		h.logger.Error("Failed to get session history", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get history"})
	}

	return c.JSON(fiber.Map{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// RegisterRoutes registers impersonation routes (admin only)
// Admin Console uses adminMiddleware which validates admin session and extracts tenant_id
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler, adminMiddleware fiber.Handler) {
	// Admin Console routes - use adminMiddleware only (validates admin session + tenant context)
	impersonate := app.Group("/admin/impersonate", adminMiddleware)
	impersonate.Post("/", h.StartImpersonation)
	impersonate.Post("/validate", h.ValidateImpersonationToken)
	impersonate.Post("/:sessionId/end", h.EndImpersonation)
	impersonate.Get("/sessions", h.GetActiveSessions)
	impersonate.Get("/history", h.GetSessionHistory)
}
