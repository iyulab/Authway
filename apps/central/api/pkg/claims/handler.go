package claims

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/tenantscope"
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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	// Parse request body
	var req UpdateClaimsRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn("Failed to parse update claims request", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	// Validate request
	if err := h.validate.Struct(&req); err != nil {
		h.logger.Warn("Invalid update claims request", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Validation failed: "+err.Error())
	}

	// Update claims
	resp, err := h.service.UpdateClaims(c.Context(), userID, tenantID, &req)
	if body, ok := reservedRefusal(err); ok {
		return c.Status(fiber.StatusBadRequest).JSON(body)
	}
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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	// Get claim key from URL parameter
	claimKey := c.Params("claim_key")
	if claimKey == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "claim_key parameter is required")
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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	// Parse request body
	var req UpdateUserClaimsRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn("Failed to parse update user claims request", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	// Validate request
	if err := h.validate.Struct(&req); err != nil {
		h.logger.Warn("Invalid update user claims request", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Validation failed: "+err.Error())
	}

	// Update user claims (no re-auth required)
	resp, err := h.service.UpdateUserClaims(c.Context(), userID, tenantID, &req)
	if body, ok := reservedRefusal(err); ok {
		return c.Status(fiber.StatusBadRequest).JSON(body)
	}
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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

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

// reservedRefusal is the 400 reserved_claim body, with the offending keys,
// for a write that named reserved claims.
func reservedRefusal(err error) (fiber.Map, bool) {
	var reservedErr *ReservedClaimError
	if !errors.As(err, &reservedErr) {
		return nil, false
	}
	return fiber.Map{"error": reservedErr.Error(), "code": "reserved_claim", "keys": reservedErr.Keys}, true
}
