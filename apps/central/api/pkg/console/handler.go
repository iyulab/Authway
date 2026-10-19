package console

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AccountLocal is the fiber.Ctx local the admin middleware fills with the
// console account (a uuid.UUID) a request was made with.
const AccountLocal = "console_account_id"

// Handler serves /api/v1/console: what the caller administers, the system
// tenant, and memberships.
type Handler struct {
	service *Service
	audit   audit.Service
	logger  *zap.Logger
}

// NewHandler returns a Handler. auditService may be nil.
func NewHandler(service *Service, auditService audit.Service, logger *zap.Logger) *Handler {
	return &Handler{service: service, audit: auditService, logger: logger}
}

// RegisterRoutes registers the console routes behind adminAuth.
func (h *Handler) RegisterRoutes(v1 fiber.Router, adminAuth fiber.Handler) {
	g := v1.Group("/console", adminAuth)
	g.Get("/me", h.Me)
	g.Get("/system-tenant", tenantscope.RequireAll(), h.GetSystemTenant)
	g.Put("/system-tenant", tenantscope.RequireAll(), h.PutSystemTenant)
	g.Get("/memberships", h.ListMemberships)
	g.Put("/memberships", h.PutMembership)
	g.Delete("/memberships/:id", h.RemoveMembership)
}

// Me handles GET /api/v1/console/me: who the caller is and what it
// administers — the console's tenant picker.
func (h *Handler) Me(c *fiber.Ctx) error {
	s, _ := tenantscope.ScopeOf(c)
	actorType, _ := c.Locals("actor_type").(string)
	body := fiber.Map{"actor_type": actorType, "operator": s.IsAll()}
	account, ok := c.Locals(AccountLocal).(uuid.UUID)
	if !ok {
		body["memberships"] = []Membership{}
		return c.JSON(body)
	}
	ms, err := h.service.MembershipsOf(c.Context(), account)
	if err != nil {
		h.logger.Error("Failed to read console memberships", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to read the memberships")
	}
	body["user_id"] = account
	body["memberships"] = ms
	return c.JSON(body)
}

// GetSystemTenant handles GET /api/v1/console/system-tenant.
func (h *Handler) GetSystemTenant(c *fiber.Ctx) error {
	t, err := h.service.SystemTenant(c.Context())
	if errors.Is(err, ErrNoSystemTenant) {
		return c.JSON(fiber.Map{"tenant": nil})
	}
	if err != nil {
		h.logger.Error("Failed to read the system tenant", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to read the system tenant")
	}
	return c.JSON(fiber.Map{"tenant": t.ToPublic()})
}

// PutSystemTenantRequest names the tenant whose accounts sign in to the console.
type PutSystemTenantRequest struct {
	TenantID string `json:"tenant_id"`
}

// PutSystemTenant handles PUT /api/v1/console/system-tenant.
func (h *Handler) PutSystemTenant(c *fiber.Ctx) error {
	var req PutSystemTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	id, err := uuid.Parse(req.TenantID)
	if err != nil {
		return apierror.Write(c, apierror.RejectField("tenant_id", "tenant_id must be a tenant id"))
	}
	t, err := h.service.SetSystemTenant(c.Context(), id)
	switch {
	case errors.Is(err, ErrUnknownTenant):
		return apierror.Write(c, apierror.RejectField("tenant_id", "no tenant has this id"))
	case errors.Is(err, ErrSystemTenantInUse):
		return apierror.RefuseWith(c, fiber.StatusConflict, "system_tenant_in_use",
			"Accounts of the current system tenant still hold memberships.",
			fiber.Map{"hint": "Remove the memberships first; they cannot be used from another tenant."})
	case err != nil:
		h.logger.Error("Failed to set the system tenant", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to set the system tenant")
	}
	h.log(c, t.ID, audit.ActionConsoleSystemTenant, "tenant", t.ID.String(), map[string]any{"slug": t.Slug})
	return c.JSON(fiber.Map{"tenant": t.ToPublic()})
}

// ListMemberships handles GET /api/v1/console/memberships: the memberships
// of the tenants the caller administers, narrowed by tenant_id. An operator
// sees every membership, operators' included.
func (h *Handler) ListMemberships(c *fiber.Ctx) error {
	s, _ := tenantscope.ScopeOf(c)
	tenants := s.Tenants()
	all := s.IsAll()
	if v := c.Query("tenant_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return apierror.RefuseWith(c, fiber.StatusBadRequest, "invalid_request", "tenant_id is not a valid id", fiber.Map{"field": "tenant_id"})
		}
		if !s.Covers(id) {
			return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "tenant not found")
		}
		tenants, all = []uuid.UUID{id}, false
	}
	ms, err := h.service.List(c.Context(), tenants, all)
	if err != nil {
		h.logger.Error("Failed to list console memberships", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to list the memberships")
	}
	return c.JSON(fiber.Map{"memberships": ms})
}

// PutMembershipRequest gives a console account a role.
type PutMembershipRequest struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

// PutMembership handles PUT /api/v1/console/memberships. An operator gives
// any role; an owner of a tenant gives roles in that tenant.
func (h *Handler) PutMembership(c *fiber.Ctx) error {
	var req PutMembershipRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return apierror.Write(c, apierror.RejectField("user_id", "user_id must be the id of an account of the system tenant"))
	}
	var tenantID *uuid.UUID
	if req.TenantID != "" {
		id, err := uuid.Parse(req.TenantID)
		if err != nil {
			return apierror.Write(c, apierror.RejectField("tenant_id", "tenant_id must be a tenant id, or absent for the operator role"))
		}
		tenantID = &id
	}
	if refusal := mayManage(c, tenantID); refusal != nil {
		return apierror.Write(c, refusal)
	}
	m, created, err := h.service.Put(c.Context(), userID, tenantID, req.Role)
	switch {
	case errors.Is(err, ErrInvalidRole):
		return apierror.Write(c, apierror.RejectField("role", "role is operator without a tenant, or owner or admin with one"))
	case errors.Is(err, ErrNoSystemTenant):
		return apierror.RefuseWith(c, fiber.StatusConflict, "no_system_tenant",
			"No tenant has been chosen for console accounts.",
			fiber.Map{"hint": "Choose one with PUT /api/v1/console/system-tenant."})
	case errors.Is(err, ErrNotConsoleAccount):
		return apierror.Write(c, apierror.RejectField("user_id", "user_id is not an account of the system tenant"))
	case errors.Is(err, ErrUnknownTenant):
		return apierror.Write(c, apierror.RejectField("tenant_id", "no tenant has this id"))
	case err != nil:
		h.logger.Error("Failed to set a console membership", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to set the membership")
	}
	h.log(c, auditTenant(m), audit.ActionConsoleMemberPut, "console_membership", m.ID.String(), map[string]any{
		"user_id": m.UserID.String(), "role": m.Role,
	})
	status := fiber.StatusOK
	if created {
		status = fiber.StatusCreated
	}
	return c.Status(status).JSON(fiber.Map{"membership": m})
}

// RemoveMembership handles DELETE /api/v1/console/memberships/:id.
func (h *Handler) RemoveMembership(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "Invalid membership id")
	}
	m, err := h.service.Get(c.Context(), id)
	if errors.Is(err, ErrNotFound) {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "membership not found")
	}
	if err != nil {
		h.logger.Error("Failed to read a console membership", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to read the membership")
	}
	if s, _ := tenantscope.ScopeOf(c); m.TenantID != nil && !s.Covers(*m.TenantID) || m.TenantID == nil && !s.IsAll() {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "membership not found")
	}
	if refusal := mayManage(c, m.TenantID); refusal != nil {
		return apierror.Write(c, refusal)
	}
	if err := h.service.Remove(c.Context(), id); err != nil && !errors.Is(err, ErrNotFound) {
		h.logger.Error("Failed to remove a console membership", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to remove the membership")
	}
	h.log(c, auditTenant(m), audit.ActionConsoleMemberRemoved, "console_membership", m.ID.String(), map[string]any{
		"user_id": m.UserID.String(), "role": m.Role,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

// mayManage refuses a caller that may not change memberships of the tenant
// (nil: the operator role): an operator may change any, an owner those of
// its tenant.
func mayManage(c *fiber.Ctx, tenantID *uuid.UUID) *apierror.Refusal {
	s, _ := tenantscope.ScopeOf(c)
	if s.IsAll() {
		return nil
	}
	if tenantID == nil {
		return apierror.Reject(fiber.StatusForbidden, "insufficient_scope", "Only an operator can give the operator role.")
	}
	role, ok := s.Role(*tenantID)
	if !ok {
		return apierror.Reject(fiber.StatusNotFound, "not_found", "tenant not found")
	}
	if role != tenantscope.RoleOwner {
		return apierror.Reject(fiber.StatusForbidden, "insufficient_scope", "Only an owner of the tenant can change who administers it.")
	}
	return nil
}

// auditTenant is the tenant whose record a membership change goes in: its
// tenant, or the platform's record for the operator role.
func auditTenant(m *Membership) uuid.UUID {
	if m.TenantID != nil {
		return *m.TenantID
	}
	return uuid.Nil
}

func (h *Handler) log(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, resourceType, resourceID string, extra map[string]any) {
	if h.audit == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, resourceType, resourceID)
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.audit.LogAsync(entry)
}
