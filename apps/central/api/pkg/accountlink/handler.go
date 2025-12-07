package accountlink

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles account linking HTTP requests
type Handler struct {
	service Service
	logger  *zap.Logger
}

// NewHandler creates a new account linking handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// GetLinkedAccounts returns all linked accounts for the authenticated user
// GET /api/v1/account/linked
func (h *Handler) GetLinkedAccounts(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	accounts, err := h.service.GetLinkedAccounts(userID)
	if err != nil {
		h.logger.Error("Failed to get linked accounts", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get linked accounts"})
	}

	response := make([]*LinkedAccountResponse, 0, len(accounts))
	for _, acc := range accounts {
		response = append(response, acc.ToResponse())
	}

	return c.JSON(fiber.Map{
		"linked_accounts": response,
		"count":           len(response),
	})
}

// GetAvailableProviders returns all providers and their linking status
// GET /api/v1/account/providers
func (h *Handler) GetAvailableProviders(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	providers, err := h.service.GetAvailableProviders(userID)
	if err != nil {
		h.logger.Error("Failed to get providers", zap.Error(err), zap.String("user_id", userID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get providers"})
	}

	return c.JSON(fiber.Map{"providers": providers})
}

// UnlinkAccount unlinks a social account from the user
// DELETE /api/v1/account/linked/:provider
func (h *Handler) UnlinkAccount(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	provider := Provider(c.Params("provider"))
	if provider == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provider is required"})
	}

	// Validate provider
	validProviders := map[Provider]bool{
		ProviderGoogle:    true,
		ProviderGitHub:    true,
		ProviderMicrosoft: true,
		ProviderApple:     true,
	}
	if !validProviders[provider] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid provider"})
	}

	if err := h.service.UnlinkAccount(userID, provider); err != nil {
		h.logger.Warn("Failed to unlink account", zap.Error(err), zap.String("user_id", userID.String()), zap.String("provider", string(provider)))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Account unlinked successfully", zap.String("user_id", userID.String()), zap.String("provider", string(provider)))
	return c.JSON(fiber.Map{
		"message":  "account unlinked successfully",
		"provider": provider,
	})
}

// RegisterRoutes registers account linking routes
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler) {
	account := app.Group("/account", authMiddleware)
	account.Get("/linked", h.GetLinkedAccounts)
	account.Get("/providers", h.GetAvailableProviders)
	account.Delete("/linked/:provider", h.UnlinkAccount)
}
