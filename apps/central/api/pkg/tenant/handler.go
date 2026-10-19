package tenant

import (
	"errors"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenantscope"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for tenant operations
type Handler struct {
	service      *Service
	validate     *validator.Validate
	auditService audit.Service
	onDeleted    []func(uuid.UUID) error
	onTurned     []func(uuid.UUID, bool) error
}

// OnTurned runs f after an update names whether a tenant is on — for what
// follows from it that this package does not own, such as the tenant's
// clients at the authorization server. It runs whenever the update names
// `active`, changed or not, so repeating a request finishes what a failure
// left undone.
func (h *Handler) OnTurned(f func(tenantID uuid.UUID, on bool) error) {
	h.onTurned = append(h.onTurned, f)
}

// OnDeleted runs f after a tenant is deleted — for what goes with it that this
// package does not own, such as the clients Authway registered for it.
func (h *Handler) OnDeleted(f func(tenantID uuid.UUID) error) {
	h.onDeleted = append(h.onDeleted, f)
}

// NewHandler creates a new tenant handler. auditService may be nil — the
// handler tolerates missing audit wiring so tests without the full DI graph
// still compile.
func NewHandler(service *Service, validate *validator.Validate, auditService audit.Service) *Handler {
	return &Handler{
		service:      service,
		validate:     validate,
		auditService: auditService,
	}
}

// logAudit emits a best-effort audit entry for a tenant admin write path.
// Same contract as other handlers: nil auditService tolerated, LogAsync may
// drop on buffer overflow — acceptable because these are non-auth paths.
func (h *Handler) logAudit(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, resourceID string, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, "tenant", resourceID)
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// RegisterRoutes registers tenant routes
// All routes require Admin API Key authentication
func (h *Handler) RegisterRoutes(app *fiber.App, adminMiddleware fiber.Handler) {
	// Each route names its own middleware: a prefix-wide Use would also run
	// in front of every /api/v1/tenants/:id/… route registered after these.
	api := app.Group("/api/v1/tenants")

	// Creating and deleting a tenant is the platform's; a tenant's owner
	// changes its settings; whoever administers it reads it.
	api.Post("/", adminMiddleware, tenantscope.RequireAll(), h.CreateTenant)
	api.Get("/", adminMiddleware, h.ListTenants)
	api.Get("/:id", adminMiddleware, tenantscope.AdmitParam("id"), h.GetTenant)
	api.Put("/:id", adminMiddleware, tenantscope.RequireOwnerOf("id"), h.UpdateTenant)
	api.Delete("/:id", adminMiddleware, tenantscope.RequireAll(), h.DeleteTenant)
}

// CreateTenant creates a new tenant
// POST /api/v1/tenants
func (h *Handler) CreateTenant(c *fiber.Ctx) error {
	var req CreateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	// Validate request
	if err := h.validate.Struct(req); err != nil {
		return apierror.Invalid(c, err)
	}

	tenant, err := h.service.CreateTenant(req)
	if err != nil {
		// Handle specific errors with appropriate status codes
		if errors.Is(err, ErrDuplicateSlug) {
			return apierror.Refuse(c, fiber.StatusConflict, "tenant_slug_taken", "A tenant with this slug already exists")
		}
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to create tenant")
	}

	h.logAudit(c, tenant.ID, audit.ActionTenantCreated, tenant.ID.String(), map[string]any{
		"slug": tenant.Slug,
		"name": tenant.Name,
	})

	return c.Status(fiber.StatusCreated).JSON(tenant.ToPublic())
}

// ListTenants lists all active tenants
// GET /api/v1/tenants
func (h *Handler) ListTenants(c *fiber.Ctx) error {
	tenants, err := h.service.ListTenants()
	if err != nil {
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", apierror.Message(err, "failed to list tenants"))
	}

	// A caller that administers some tenants sees those.
	scope, _ := tenantscope.ScopeOf(c)
	publicTenants := make([]PublicTenant, 0, len(tenants))
	for _, tenant := range tenants {
		if scope.Covers(tenant.ID) {
			publicTenants = append(publicTenants, tenant.ToPublic())
		}
	}

	return c.JSON(publicTenants)
}

// GetTenant retrieves a tenant by ID
// GET /api/v1/tenants/:id
func (h *Handler) GetTenant(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid tenant ID format")
	}

	tenant, err := h.service.GetTenantByID(id)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", apierror.Message(err, "tenant not found"))
	}

	return c.JSON(tenant.ToPublic())
}

// UpdateTenant updates a tenant
// PUT /api/v1/tenants/:id
func (h *Handler) UpdateTenant(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid tenant ID format")
	}

	var req UpdateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}

	// Validate request
	if err := h.validate.Struct(req); err != nil {
		return apierror.Invalid(c, err)
	}
	if req.Logo != nil && *req.Logo != "" && h.validate.Var(*req.Logo, "url") != nil {
		return apierror.Write(c, apierror.RejectField("logo", "logo must be an absolute URL, or empty to clear it"))
	}
	if req.PrimaryColor != nil && *req.PrimaryColor != "" && h.validate.Var(*req.PrimaryColor, "hexcolor") != nil {
		return apierror.Write(c, apierror.RejectField("primary_color", "primary_color must be a hex color such as #4F46E5, or empty to clear it"))
	}

	// Before-state snapshot for audit diff. Miss surfaces as the same 404
	// UpdateTenant would raise, so the service owns the canonical error path.
	beforeTenant, _ := h.service.GetTenantByID(id)

	// Turning a tenant off or on is the platform's, not its owner's. Sending
	// the value it already has changes nothing and is let through.
	if scope, _ := tenantscope.ScopeOf(c); req.Active != nil && !scope.IsAll() &&
		(beforeTenant == nil || *req.Active != beforeTenant.Active) {
		return apierror.RefuseWith(c, fiber.StatusForbidden, "insufficient_scope",
			"Only an operator of this Authway can turn a tenant off or on.", fiber.Map{"field": "active"})
	}

	tenant, err := h.service.UpdateTenant(id, req)
	if err != nil {
		// Handle specific errors with appropriate status codes
		if errors.Is(err, ErrNotFound) {
			return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Tenant not found")
		}
		if errors.Is(err, ErrCannotDeactivateDefault) {
			return apierror.Refuse(c, fiber.StatusForbidden, "default_tenant_protected", "Cannot deactivate the default tenant")
		}
		if errors.Is(err, ErrSystemTenantProtected) {
			return apierror.RefuseWith(c, fiber.StatusConflict, "system_tenant", "The tenant whose accounts sign in to the console cannot be turned off.", fiber.Map{"field": "active"})
		}
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to update tenant")
	}

	// Diff-only details: flagging the *changed* fields makes the audit row
	// useful for forensics without ballooning storage on full snapshots.
	auditDetails := map[string]any{
		"slug": tenant.Slug,
	}
	if beforeTenant != nil {
		if beforeTenant.Name != tenant.Name {
			auditDetails["name_before"] = beforeTenant.Name
			auditDetails["name_after"] = tenant.Name
		}
		if beforeTenant.Active != tenant.Active {
			auditDetails["active_before"] = beforeTenant.Active
			auditDetails["active_after"] = tenant.Active
		}
	}
	h.logAudit(c, tenant.ID, audit.ActionTenantUpdated, tenant.ID.String(), auditDetails)

	if req.Active != nil {
		for _, f := range h.onTurned {
			if err := f(tenant.ID, tenant.Active); err != nil {
				if logger, ok := c.Locals("logger").(*zap.Logger); ok {
					logger.Error("A tenant's applications were not all brought in line with it", zap.String("tenant_id", tenant.ID.String()), zap.Error(err))
				}
				return apierror.RefuseWith(c, fiber.StatusBadGateway, "upstream_sync_failed",
					"The tenant was saved, but not every one of its applications was brought in line at the authorization server.",
					fiber.Map{"hint": "Send the same request again."})
			}
		}
	}

	return c.JSON(tenant.ToPublic())
}

// DeleteTenant soft deletes a tenant
// DELETE /api/v1/tenants/:id
func (h *Handler) DeleteTenant(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid tenant ID format")
	}

	// Snapshot before deletion — without this the audit entry cannot answer
	// "what was the slug/name of the deleted tenant?" (soft-delete still hides
	// it from the default query scope).
	beforeTenant, _ := h.service.GetTenantByID(id)

	if err := h.service.DeleteTenant(id); err != nil {
		// Handle specific errors with appropriate status codes
		if errors.Is(err, ErrNotFound) {
			return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Tenant not found")
		}
		if errors.Is(err, ErrCannotDeleteDefault) {
			return apierror.Refuse(c, fiber.StatusForbidden, "default_tenant_protected", "Cannot delete the default tenant")
		}
		if errors.Is(err, ErrSystemTenantProtected) {
			return apierror.Refuse(c, fiber.StatusConflict, "system_tenant", "The tenant whose accounts sign in to the console cannot be deleted.")
		}
		if errors.Is(err, ErrHasUsers) {
			return apierror.Refuse(c, fiber.StatusConflict, "tenant_has_users", "Cannot delete tenant with existing users")
		}
		if errors.Is(err, ErrHasClients) {
			return apierror.Refuse(c, fiber.StatusConflict, "tenant_has_clients", "Cannot delete tenant with existing clients")
		}
		if errors.Is(err, ErrHasServiceClients) {
			return apierror.Refuse(c, fiber.StatusConflict, "tenant_has_service_clients", "Cannot delete tenant with active service clients")
		}
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to delete tenant")
	}

	for _, f := range h.onDeleted {
		if err := f(id); err != nil {
			// The tenant is gone either way; what stayed behind is logged to clear.
			if logger, ok := c.Locals("logger").(*zap.Logger); ok {
				logger.Error("Something of a deleted tenant was not removed", zap.String("tenant_id", id.String()), zap.Error(err))
			}
		}
	}

	if beforeTenant != nil {
		h.logAudit(c, beforeTenant.ID, audit.ActionTenantDeleted, beforeTenant.ID.String(), map[string]any{
			"slug": beforeTenant.Slug,
			"name": beforeTenant.Name,
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
