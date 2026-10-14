package terms

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tenantscope"
	"authway/apps/central/api/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TenantFinder finds the tenant a request names.
type TenantFinder interface {
	GetTenantByID(id uuid.UUID) (*tenant.Tenant, error)
}

// UserFinder finds the user a request names.
type UserFinder interface {
	GetByID(id uuid.UUID) (*user.User, error)
}

// Handler serves a tenant's documents and a user's acceptances to an
// administrator or the tenant's service credential. The route's middleware
// authenticates the caller; a service credential acts on its own tenant
// only, and anything of another tenant is not found.
type Handler struct {
	service Service
	tenants TenantFinder
	users   UserFinder
	audit   audit.Service
	logger  *zap.Logger
}

// NewHandler returns a Handler. auditService may be nil.
func NewHandler(service Service, tenants TenantFinder, users UserFinder, auditService audit.Service, logger *zap.Logger) *Handler {
	return &Handler{service: service, tenants: tenants, users: users, audit: auditService, logger: logger}
}

// tenantOf resolves the tenant the path names and admits it for the caller.
func (h *Handler) tenantOf(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, apierror.Reject(fiber.StatusBadRequest, "invalid_request", "Invalid tenant ID")
	}
	if err := tenantscope.Admit(c, id, "Tenant not found"); err != nil {
		return uuid.Nil, err
	}
	if _, err := h.tenants.GetTenantByID(id); err != nil {
		return uuid.Nil, apierror.Reject(fiber.StatusNotFound, "not_found", "Tenant not found")
	}
	return id, nil
}

// List handles GET /api/v1/tenants/:id/terms.
func (h *Handler) List(c *fiber.Ctx) error {
	tenantID, err := h.tenantOf(c)
	if err != nil {
		return err
	}
	docs, err := h.service.Current(c.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to read tenant documents", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to get the documents")
	}
	views := make([]DocumentView, 0, len(docs))
	for _, d := range docs {
		views = append(views, d.View())
	}
	return c.JSON(fiber.Map{"terms": views})
}

// Publish handles PUT /api/v1/tenants/:id/terms/:kind.
func (h *Handler) Publish(c *fiber.Ctx) error {
	tenantID, err := h.tenantOf(c)
	if err != nil {
		return err
	}
	var req PublishRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	doc, created, err := h.service.Publish(c.Context(), tenantID, c.Params("kind"), req)
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", invalid.Reason)
	case err != nil:
		h.logger.Error("Failed to publish a tenant document", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to publish the document")
	}
	h.log(c, tenantID, audit.ActionTenantTermsPublished, doc.Kind, map[string]any{
		"kind": doc.Kind, "version": doc.Version, "new_version": created,
	})
	status := fiber.StatusOK
	if created {
		status = fiber.StatusCreated
	}
	return c.Status(status).JSON(doc.View())
}

// Retire handles DELETE /api/v1/tenants/:id/terms/:kind.
func (h *Handler) Retire(c *fiber.Ctx) error {
	tenantID, err := h.tenantOf(c)
	if err != nil {
		return err
	}
	kind := c.Params("kind")
	err = h.service.Retire(c.Context(), tenantID, kind)
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", invalid.Reason)
	case errors.Is(err, ErrNotFound):
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "The tenant does not ask for that document")
	case err != nil:
		h.logger.Error("Failed to retire a tenant document", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to retire the document")
	}
	h.log(c, tenantID, audit.ActionTenantTermsRetired, kind, map[string]any{"kind": kind})
	return c.SendStatus(fiber.StatusNoContent)
}

// Acceptances handles GET /api/v1/users/:id/terms-acceptances.
func (h *Handler) Acceptances(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid user ID")
	}
	u, err := h.users.GetByID(id)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "User not found")
	}
	if err := tenantscope.Admit(c, u.TenantID, "User not found"); err != nil {
		return err
	}
	list, err := h.service.Acceptances(c.Context(), u.ID)
	if err != nil {
		h.logger.Error("Failed to read acceptances", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to get the acceptances")
	}
	return c.JSON(fiber.Map{"acceptances": list})
}

func (h *Handler) log(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, kind string, details map[string]any) {
	if h.audit == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, "terms", kind)
	for k, v := range details {
		entry.Details[k] = v
	}
	h.audit.LogAsync(entry)
}
