package handler

import (
	"strconv"

	"authway/src/server/internal/service"
	"authway/src/server/pkg/user"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type UserHandler struct {
	services  *service.Services
	logger    *zap.Logger
	validator *validator.Validate
}

func NewUserHandler(services *service.Services, logger *zap.Logger) *UserHandler {
	return &UserHandler{
		services:  services,
		logger:    logger,
		validator: validator.New(),
	}
}

// List handles listing users with pagination
func (h *UserHandler) List(c *fiber.Ctx) error {
	// Parse query parameters
	limitStr := c.Query("limit", "20")
	offsetStr := c.Query("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	users, total, err := h.services.UserService.List(limit, offset)
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve users")
	}

	// Convert to public user objects
	publicUsers := make([]user.PublicUser, len(users))
	for i, u := range users {
		publicUsers[i] = u.ToPublic()
	}

	return c.JSON(fiber.Map{
		"users":  publicUsers,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Get handles getting a specific user by ID
func (h *UserHandler) Get(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

	foundUser, err := h.services.UserService.GetByID(id)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}

	return c.JSON(fiber.Map{
		"user": foundUser.ToPublic(),
	})
}

// Update handles updating user information
func (h *UserHandler) Update(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

	var req user.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	updatedUser, err := h.services.UserService.Update(id, &req)
	if err != nil {
		h.logger.Error("Failed to update user", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("User updated successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message": "User updated successfully",
		"user":    updatedUser.ToPublic(),
	})
}

// Delete handles deleting a user
func (h *UserHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

	if err := h.services.UserService.Delete(id); err != nil {
		h.logger.Error("Failed to delete user", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("User deleted successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message": "User deleted successfully",
	})
}
