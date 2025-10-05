package admin

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type Handler struct {
	service Service
	logger  *zap.Logger
	version string
}

func NewHandler(service Service, logger *zap.Logger, version string) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
		version: version,
	}
}

// RegisterRoutes registers admin console routes
func (h *Handler) RegisterRoutes(app *fiber.App) {
	// Public routes (no auth required)
	admin := app.Group("/admin")
	admin.Post("/login", h.Login)
	admin.Get("/info", h.Info)

	// Protected routes (admin session required)
	admin.Post("/logout", h.AdminAuthMiddleware(), h.Logout)
	admin.Get("/validate", h.AdminAuthMiddleware(), h.Validate)
}

// Login authenticates admin and returns session token
func (h *Handler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password is required",
		})
	}

	session, err := h.service.Authenticate(req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid password",
		})
	}

	return c.JSON(LoginResponse{
		Token:     session.Token,
		ExpiresAt: session.ExpiresAt,
	})
}

// Logout terminates admin session
func (h *Handler) Logout(c *fiber.Ctx) error {
	token := h.extractToken(c)
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "No token provided",
		})
	}

	if err := h.service.Logout(token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

// Validate checks if current session is valid
func (h *Handler) Validate(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"valid": true,
		"info": AdminInfo{
			Authenticated: true,
			Version:       h.version,
		},
	})
}

// Info returns admin console information (public)
func (h *Handler) Info(c *fiber.Ctx) error {
	return c.JSON(AdminInfo{
		Authenticated: false,
		Version:       h.version,
	})
}

// AdminAuthMiddleware validates admin session token
func (h *Handler) AdminAuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := h.extractToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No authorization token provided",
			})
		}

		valid, err := h.service.ValidateToken(token)
		if err != nil {
			h.logger.Error("Failed to validate admin token", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to validate session",
			})
		}

		if !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired session",
			})
		}

		c.Locals("admin_authenticated", true)
		return c.Next()
	}
}

// extractToken extracts bearer token from Authorization header
func (h *Handler) extractToken(c *fiber.Ctx) string {
	auth := c.Get("Authorization")
	if auth == "" {
		return ""
	}

	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}

	return strings.TrimPrefix(auth, "Bearer ")
}
