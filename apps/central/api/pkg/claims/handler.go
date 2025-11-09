package claims

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for claims management
type Handler struct {
	service  Service
	logger   *zap.Logger
	validate *validator.Validate
}

// NewHandler creates a new claims handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service:  service,
		logger:   logger,
		validate: validator.New(),
	}
}

// HandleUpdateClaims handles POST /api/v1/claims/update
func (h *Handler) HandleUpdateClaims(c *fiber.Ctx) error {
	// Get user ID and tenant ID from context (set by auth middleware)
	userID := c.Locals("user_id").(uuid.UUID)
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	// Parse request body
	var req UpdateClaimsRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn("Failed to parse update claims request", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if err := h.validate.Struct(&req); err != nil {
		h.logger.Warn("Invalid update claims request", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"details": err.Error(),
		})
	}

	// Update claims
	resp, err := h.service.UpdateClaims(c.Context(), userID, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update claims", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update claims",
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// HandleGetClaims handles GET /api/v1/claims
func (h *Handler) HandleGetClaims(c *fiber.Ctx) error {
	// Get user ID and tenant ID from context (set by auth middleware)
	userID := c.Locals("user_id").(uuid.UUID)
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	// Get claims
	resp, err := h.service.GetClaims(c.Context(), userID, tenantID)
	if err != nil {
		h.logger.Error("Failed to get claims", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get claims",
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// HandleDeleteClaim handles DELETE /api/v1/claims/:claim_key
func (h *Handler) HandleDeleteClaim(c *fiber.Ctx) error {
	// Get user ID and tenant ID from context (set by auth middleware)
	userID := c.Locals("user_id").(uuid.UUID)
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	// Get claim key from URL parameter
	claimKey := c.Params("claim_key")
	if claimKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "claim_key parameter is required",
		})
	}

	// Delete claim
	resp, err := h.service.DeleteClaim(c.Context(), userID, tenantID, claimKey)
	if err != nil {
		h.logger.Error("Failed to delete claim",
			zap.String("claim_key", claimKey),
			zap.Error(err),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete claim",
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// HandleUpdateUserClaims handles PATCH /api/v1/claims/user (no re-authentication)
func (h *Handler) HandleUpdateUserClaims(c *fiber.Ctx) error {
	// Get user ID and tenant ID from context (set by auth middleware)
	userID := c.Locals("user_id").(uuid.UUID)
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	// Parse request body
	var req UpdateUserClaimsRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn("Failed to parse update user claims request", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if err := h.validate.Struct(&req); err != nil {
		h.logger.Warn("Invalid update user claims request", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"details": err.Error(),
		})
	}

	// Update user claims (no re-auth required)
	resp, err := h.service.UpdateUserClaims(c.Context(), userID, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update user claims", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user claims",
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// HandleGetUserClaims handles GET /api/v1/claims/user
func (h *Handler) HandleGetUserClaims(c *fiber.Ctx) error {
	// Get user ID and tenant ID from context (set by auth middleware)
	userID := c.Locals("user_id").(uuid.UUID)
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	// Get user claims
	resp, err := h.service.GetUserClaims(c.Context(), userID, tenantID)
	if err != nil {
		h.logger.Error("Failed to get user claims", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get user claims",
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}
