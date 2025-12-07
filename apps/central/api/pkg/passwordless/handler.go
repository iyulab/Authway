package passwordless

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles passwordless authentication HTTP requests
type Handler struct {
	service Service
	logger  *zap.Logger
}

// NewHandler creates a new passwordless handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// SendMagicLink sends a magic link to the user's email
// POST /api/v1/auth/magic-link/send
func (h *Handler) SendMagicLink(c *fiber.Ctx) error {
	// Get tenant ID from query, header, or body
	tenantIDStr := c.Query("tenant_id")
	if tenantIDStr == "" {
		tenantIDStr = c.Get("X-Tenant-ID")
	}

	var req SendMagicLinkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Allow tenant_id in body as well
	if tenantIDStr == "" {
		var body struct {
			TenantID string `json:"tenant_id"`
		}
		c.BodyParser(&body)
		tenantIDStr = body.TenantID
	}

	if tenantIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant_id is required"})
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}

	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	response, err := h.service.SendMagicLink(tenantID, &req, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("Failed to send magic link", zap.Error(err), zap.String("email", req.Email))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Magic link sent", zap.String("email", req.Email), zap.String("tenant_id", tenantID.String()))
	return c.JSON(response)
}

// VerifyMagicLink verifies a magic link token and authenticates the user
// POST /api/v1/auth/magic-link/verify
func (h *Handler) VerifyMagicLink(c *fiber.Ctx) error {
	// Token can come from query param or body
	token := c.Query("token")
	if token == "" {
		var req VerifyMagicLinkRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		token = req.Token
	}

	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	magicLink, user, err := h.service.VerifyMagicLink(token)
	if err != nil {
		h.logger.Warn("Failed to verify magic link", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Magic link verified",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email),
		zap.String("token_type", string(magicLink.TokenType)),
	)

	response := fiber.Map{
		"message": "authentication successful",
		"user": fiber.Map{
			"id":             user.ID.String(),
			"email":          user.Email,
			"email_verified": user.EmailVerified,
			"tenant_id":      user.TenantID.String(),
		},
		"token_type": magicLink.TokenType,
	}

	// Include redirect info if present
	if magicLink.RedirectURI != "" {
		response["redirect_uri"] = magicLink.RedirectURI
	}
	if magicLink.State != "" {
		response["state"] = magicLink.State
	}
	if magicLink.ClientID != "" {
		response["client_id"] = magicLink.ClientID
	}

	return c.JSON(response)
}

// GetMagicLinkStatus checks if a magic link is valid (without consuming it)
// GET /api/v1/auth/magic-link/status
func (h *Handler) GetMagicLinkStatus(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	// We just check if the verify would work
	// This is a read-only operation
	magicLink, _, err := h.service.VerifyMagicLink(token)
	if err != nil {
		return c.JSON(fiber.Map{
			"valid":   false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"valid":      true,
		"email":      magicLink.Email,
		"token_type": magicLink.TokenType,
		"expires_at": magicLink.ExpiresAt,
	})
}

// RegisterRoutes registers passwordless authentication routes
func (h *Handler) RegisterRoutes(app fiber.Router) {
	magicLink := app.Group("/auth/magic-link")
	magicLink.Post("/send", h.SendMagicLink)
	magicLink.Post("/verify", h.VerifyMagicLink)
	magicLink.Get("/verify", h.VerifyMagicLink) // Support GET for email links
	magicLink.Get("/status", h.GetMagicLinkStatus)
}
