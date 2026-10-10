package handler

import (
	"authway/apps/central/api/pkg/tenantscope"
	"strconv"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/service"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/user"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type UserHandler struct {
	services     *service.Services
	hydraClient  *hydra.Client
	logger       *zap.Logger
	validator    *validator.Validate
	auditService audit.Service
}

func NewUserHandler(services *service.Services, hydraClient *hydra.Client, logger *zap.Logger, auditService audit.Service) *UserHandler {
	return &UserHandler{
		services:     services,
		hydraClient:  hydraClient,
		logger:       logger,
		validator:    validator.New(),
		auditService: auditService,
	}
}

// logAudit emits a best-effort audit entry for a user admin write path. See
// ClientHandler.logAudit — same contract: nil auditService is tolerated, and
// LogAsync may drop on buffer overflow (not acceptable for auth-failure paths,
// which use sync Log instead).
func (h *UserHandler) logAudit(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, resourceID string, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, "user", resourceID)
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// List handles listing users with pagination and optional tenant filtering
func (h *UserHandler) List(c *fiber.Ctx) error {
	// Parse query parameters
	limitStr := c.Query("limit", "20")
	offsetStr := c.Query("offset", "0")
	tenantIDStr := c.Query("tenant_id", "")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	var users []*user.User
	var total int64

	// If tenant_id is provided, filter by tenant
	if tenantIDStr != "" {
		tenantID, err := uuid.Parse(tenantIDStr)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid tenant ID")
		}
		users, total, err = h.services.UserService.GetByTenant(tenantID, limit, offset)
		if err != nil {
			h.logger.Error("Failed to list users by tenant", zap.Error(err), zap.String("tenant_id", tenantIDStr))
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve users")
		}
	} else {
		users, total, err = h.services.UserService.List(limit, offset)
		if err != nil {
			h.logger.Error("Failed to list users", zap.Error(err))
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve users")
		}
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

// admit refuses, as not found, a user of another tenant than the request
// names. A missing user is left to the caller's own answer.
func (h *UserHandler) admit(c *fiber.Ctx, id uuid.UUID) error {
	found, err := h.services.UserService.GetByID(id)
	if err != nil {
		return nil
	}
	return tenantscope.Admit(c, found.TenantID, "User not found")
}

// Get handles getting a specific user by ID
func (h *UserHandler) Get(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	if err := h.admit(c, id); err != nil {
		return err
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
	if err := h.admit(c, id); err != nil {
		return err
	}

	var req user.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	// Before-state snapshot for audit diff. A miss surfaces as the same 404
	// Update would raise, so we let Update own the canonical error path.
	beforeUser, _ := h.services.UserService.GetByID(id)

	updatedUser, err := h.services.UserService.Update(id, &req)
	if err != nil {
		h.logger.Error("Failed to update user", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "user not found"))
	}

	h.logger.Info("User updated successfully", zap.String("id", idStr))

	auditDetails := map[string]any{
		"email": updatedUser.Email,
	}
	if beforeUser != nil {
		if !stringPtrEqual(beforeUser.Name, updatedUser.Name) {
			auditDetails["name_before"] = stringPtrValue(beforeUser.Name)
			auditDetails["name_after"] = stringPtrValue(updatedUser.Name)
		}
		if !stringPtrEqual(beforeUser.AvatarURL, updatedUser.AvatarURL) {
			auditDetails["avatar_url_before"] = stringPtrValue(beforeUser.AvatarURL)
			auditDetails["avatar_url_after"] = stringPtrValue(updatedUser.AvatarURL)
		}
	}
	h.logAudit(c, updatedUser.TenantID, audit.ActionUserUpdated, updatedUser.ID.String(), auditDetails)

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
	if err := h.admit(c, id); err != nil {
		return err
	}

	// Snapshot before deletion — the row disappears, so without this the audit
	// entry cannot answer "which tenant did the deleted user belong to?"
	beforeUser, err := h.services.UserService.GetByID(id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "user not found"))
	}

	// The user's sessions and tokens end with the account.
	if err := removeAccount(h.hydraClient, h.services.UserService, id); err != nil {
		h.logger.Error("Failed to delete user", zap.Error(err), zap.String("id", idStr))
		return apierror.Refuse(c, fiber.StatusBadGateway, "authorization_server_unavailable", "The user could not be deleted. Try again shortly.")
	}

	h.logger.Info("User deleted successfully", zap.String("id", idStr))

	if beforeUser != nil {
		h.logAudit(c, beforeUser.TenantID, audit.ActionUserDeleted, beforeUser.ID.String(), map[string]any{
			"email": beforeUser.Email,
		})
	}

	return c.JSON(fiber.Map{
		"message": "User deleted successfully",
	})
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func stringPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
