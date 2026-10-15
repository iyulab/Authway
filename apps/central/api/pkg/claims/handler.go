package claims

import (
	"errors"
	"sort"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenantscope"
	"authway/apps/central/api/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserFinder finds the user whose claims a request names.
type UserFinder interface {
	GetByID(id uuid.UUID) (*user.User, error)
}

// Handler serves a user's stored claims to an administrator or a tenant's
// service credential (GET/PATCH /api/v1/users/{id}/claims). The route's
// middleware authenticates the caller; a service credential's tenant is the
// request's tenant, so a user of another tenant is not found.
type Handler struct {
	service Service
	users   UserFinder
	audit   audit.Service
	logger  *zap.Logger
}

// NewHandler creates a new claims handler. auditService may be nil.
func NewHandler(service Service, users UserFinder, auditService audit.Service, logger *zap.Logger) *Handler {
	return &Handler{service: service, users: users, audit: auditService, logger: logger}
}

// target resolves the user the request names and admits it for the caller's
// tenant.
func (h *Handler) target(c *fiber.Ctx) (*user.User, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return nil, apierror.Reject(fiber.StatusBadRequest, "invalid_request", "Invalid user ID")
	}
	found, err := h.users.GetByID(id)
	if err != nil {
		return nil, apierror.Reject(fiber.StatusNotFound, "not_found", "User not found")
	}
	if err := tenantscope.Admit(c, found.TenantID, "User not found"); err != nil {
		return nil, err
	}
	return found, nil
}

// List handles GET /api/v1/users/:id/claims.
func (h *Handler) List(c *fiber.Ctx) error {
	u, err := h.target(c)
	if err != nil {
		return err
	}
	stored, err := h.service.List(c.Context(), u.ID, u.TenantID)
	if err != nil {
		h.logger.Error("Failed to list claims", zap.Error(err), zap.String("user_id", u.ID.String()))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to get claims")
	}
	return c.JSON(ClaimsResponse{Claims: stored})
}

// Patch handles PATCH /api/v1/users/:id/claims.
func (h *Handler) Patch(c *fiber.Ctx) error {
	u, err := h.target(c)
	if err != nil {
		return err
	}
	var req PatchClaimsRequest
	if err := c.BodyParser(&req); err != nil || req.Claims == nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", `The body must be {"claims": {…}}`)
	}

	stored, err := h.service.Patch(c.Context(), u.ID, u.TenantID, req.Claims)
	var reservedErr *ReservedClaimError
	var invalidErr *InvalidClaimError
	var tooLargeErr *ClaimsTooLargeError
	switch {
	case errors.As(err, &reservedErr):
		return apierror.RefuseWith(c, fiber.StatusBadRequest, "reserved_claim", reservedErr.Error(), fiber.Map{"keys": reservedErr.Keys})
	case errors.As(err, &invalidErr):
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", invalidErr.Error())
	case errors.As(err, &tooLargeErr):
		return apierror.Refuse(c, fiber.StatusRequestEntityTooLarge, "claims_too_large", tooLargeErr.Error())
	case err != nil:
		h.logger.Error("Failed to patch claims", zap.Error(err), zap.String("user_id", u.ID.String()))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to update claims")
	}

	h.logPatch(c, u, req.Claims)
	return c.JSON(ClaimsResponse{Claims: stored})
}

// logPatch records which claims were set and removed — the names, not the
// values, which may be anything the caller chose to store.
func (h *Handler) logPatch(c *fiber.Ctx, u *user.User, patch map[string]any) {
	if h.audit == nil {
		return
	}
	set, removed := []string{}, []string{}
	for key, value := range patch {
		if value == nil {
			removed = append(removed, key)
		} else {
			set = append(set, key)
		}
	}
	sort.Strings(set)
	sort.Strings(removed)
	entry := audit.EntryFromFiber(c, u.TenantID, audit.ActionUserClaimsUpdated, "user", u.ID.String())
	entry.Details["set"] = set
	entry.Details["removed"] = removed
	h.audit.LogAsync(entry)
}
