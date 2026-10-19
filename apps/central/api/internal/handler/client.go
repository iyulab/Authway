package handler

import (
	"authway/apps/central/api/pkg/tenantscope"
	"errors"
	"strconv"
	"strings"

	"authway/apps/central/api/internal/service"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ClientHandler struct {
	services     *service.Services
	logger       *zap.Logger
	validator    *validator.Validate
	auditService audit.Service
}

func NewClientHandler(services *service.Services, logger *zap.Logger, auditService audit.Service) *ClientHandler {
	return &ClientHandler{
		services:     services,
		logger:       logger,
		validator:    validator.New(),
		auditService: auditService,
	}
}

// logAudit emits an audit log entry for a client write path. audit is best-effort
// (LogAsync drops on buffer overflow) — never block the response on audit I/O.
// A nil auditService (e.g. in older tests) is tolerated so handler can be
// exercised without the full DI graph.
func (h *ClientHandler) logAudit(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, resourceID string, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, "client", resourceID)
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// List handles listing OAuth clients with pagination
// Supports optional tenant_id query parameter to filter clients by tenant
func (h *ClientHandler) List(c *fiber.Ctx) error {
	// Parse query parameters
	limitStr := c.Query("limit", "20")
	offsetStr := c.Query("offset", "0")
	// The tenant a call names in X-Tenant-ID, as everywhere else in the admin
	// API; tenant_id in the query is the older way of saying it. A call that
	// names none lists every tenant's clients — when its caller administers every
	// tenant; any other caller names one of its tenants.
	tenantID, named, err := tenantscope.Named(c)
	if err != nil {
		return err
	}
	if scope, _ := tenantscope.ScopeOf(c); !named && !scope.IsAll() {
		return apierror.Refuse(c, fiber.StatusBadRequest, "tenant_required",
			"name the tenant with the "+tenantscope.Header+" header or the tenant_id query parameter")
	}

	limit, err := strconv.Atoi(limitStr)
	switch {
	case err != nil || limit < 1:
		limit = 20
	case limit > 100:
		// A page holds at most 100; asking for more gets the most, not the default.
		limit = 100
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	var clients []*client.Client
	var total int64

	// If tenant_id is provided, filter by tenant
	if named {

		clients, total, err = h.services.ClientService.GetByTenant(tenantID, limit, offset)
		if err != nil {
			h.logger.Error("Failed to list clients by tenant", zap.Error(err), zap.String("tenant_id", tenantID.String()))
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve clients")
		}
	} else {
		// No tenant filter, list all clients
		clients, total, err = h.services.ClientService.List(limit, offset)
		if err != nil {
			h.logger.Error("Failed to list clients", zap.Error(err))
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve clients")
		}
	}

	// Convert to public client objects
	publicClients := make([]client.PublicClient, len(clients))
	for i, cl := range clients {
		publicClients[i] = cl.ToPublic()
	}

	return c.JSON(fiber.Map{
		"clients": publicClients,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// Create handles creating a new OAuth client. actor_type == "service_client"
// (set by admin.Handler.GetClientAuth) routes to createScoped, which accepts
// only a whitelisted field subset and forces tenant_id from the validated
// credential; every other caller (admin API key / admin session — no
// actor_type Local, or actor_type != "service_client") keeps the existing
// full-request behavior unchanged.
func (h *ClientHandler) Create(c *fiber.Ctx) error {
	if actorType, _ := c.Locals("actor_type").(string); actorType == "service_client" {
		return h.createScoped(c)
	}

	var req client.CreateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return apierror.Invalid(c, err)
	}
	// The tenant is named in the body, by the X-Tenant-ID header as for the
	// tenant's other admin calls, or by both — and then they agree: a client
	// is never registered in a tenant other than the one the request is about.
	named, ok, err := tenantscope.Named(c)
	if err != nil {
		return err
	}
	switch {
	case ok && req.TenantID == "":
		req.TenantID = named.String()
	case ok && !strings.EqualFold(req.TenantID, named.String()):
		return apierror.RefuseWith(c, fiber.StatusBadRequest, "invalid_request",
			"tenant_id names a different tenant than the "+tenantscope.Header+" header",
			fiber.Map{"field": "tenant_id", "hint": "Name the tenant once, or name the same one in both."})
	case !ok && req.TenantID == "":
		return apierror.RefuseWith(c, fiber.StatusBadRequest, "tenant_required",
			"name the tenant with tenant_id or the "+tenantscope.Header+" header", fiber.Map{"field": "tenant_id"})
	}
	// The tenant named in the body alone is held to the caller's scope too.
	if tid, err := uuid.Parse(req.TenantID); err == nil {
		if err := tenantscope.Admit(c, tid, "tenant not found"); err != nil {
			return err
		}
	}

	newClient, credentials, err := h.services.ClientService.Create(&req)
	if err != nil {
		// Surface client-config violations as 400 with a stable code/hint payload
		// so SDKs and the Admin Console can render targeted UX.
		var refusal *apierror.Refusal
		if errors.As(err, &refusal) {
			return apierror.Write(c, refusal)
		}
		h.logger.Error("Failed to create client", zap.Error(err), zap.String("name", req.Name))
		return fiber.NewError(fiber.StatusInternalServerError, apierror.Message(err, "failed to create client"))
	}

	h.logger.Info("Client created successfully", zap.String("client_id", newClient.ClientID))

	h.logAudit(c, newClient.TenantID, audit.ActionClientCreated, newClient.ID.String(), map[string]any{
		"client_id":     newClient.ClientID,
		"name":          newClient.Name,
		"public":        newClient.Public,
		"grant_types":   []string(newClient.GrantTypes),
		"redirect_uris": []string(newClient.RedirectURIs),
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":     "Client created successfully",
		"client":      newClient.ToPublic(),
		"credentials": credentials,
	})
}

// createScoped is the service_client branch of Create — see the doc comment
// above.
func (h *ClientHandler) createScoped(c *fiber.Ctx) error {
	var scoped client.ScopedCreateClientRequest
	if err := c.BodyParser(&scoped); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if err := h.validator.Struct(&scoped); err != nil {
		return apierror.Invalid(c, err)
	}

	tenantID, _ := c.Locals("tenant_id").(string)
	req := scoped.ToCreateClientRequest(tenantID)

	newClient, credentials, err := h.services.ClientService.Create(req)
	if err != nil {
		var refusal *apierror.Refusal
		if errors.As(err, &refusal) {
			return apierror.Write(c, refusal)
		}
		h.logger.Error("Failed to create client (scoped)", zap.Error(err), zap.String("name", scoped.Name), zap.String("tenant_id", tenantID))
		return fiber.NewError(fiber.StatusInternalServerError, apierror.Message(err, "failed to create client"))
	}

	h.logger.Info("Client created successfully via service_client", zap.String("client_id", newClient.ClientID), zap.String("tenant_id", tenantID))

	h.logAudit(c, newClient.TenantID, audit.ActionClientCreated, newClient.ID.String(), map[string]any{
		"client_id":     newClient.ClientID,
		"name":          newClient.Name,
		"public":        newClient.Public,
		"grant_types":   []string(newClient.GrantTypes),
		"redirect_uris": []string(newClient.RedirectURIs),
		"actor_type":    "service_client",
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":     "Client created successfully",
		"client":      newClient.ToPublic(),
		"credentials": credentials,
	})
}

// admit refuses, as not found, a client of another tenant than the request
// names. A missing client is left to the caller's own answer.
func (h *ClientHandler) admit(c *fiber.Ctx, id uuid.UUID) error {
	found, err := h.services.ClientService.GetByID(id)
	if err != nil {
		return nil
	}
	return tenantscope.Admit(c, found.TenantID, "Client not found")
}

// Get handles getting a specific OAuth client by ID
func (h *ClientHandler) Get(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}
	if err := h.admit(c, id); err != nil {
		return err
	}

	foundClient, err := h.services.ClientService.GetByID(id)
	if err != nil {
		h.logger.Error("Failed to get client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, "Client not found")
	}

	return c.JSON(fiber.Map{
		"client": foundClient.ToPublic(),
	})
}

// Update handles updating OAuth client information
func (h *ClientHandler) Update(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}
	if err := h.admit(c, id); err != nil {
		return err
	}

	var req client.UpdateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return apierror.Invalid(c, err)
	}

	// Capture before-state for audit diff. Missing record surfaces as the
	// same 404 the Update call would raise, so we only take the snapshot on
	// success and let Update own the canonical error path.
	beforeClient, _ := h.services.ClientService.GetByID(id)

	updatedClient, syncStatus, err := h.services.ClientService.Update(id, &req)
	if errors.Is(err, client.ErrSystemClient) {
		return refuseSystemClient(c)
	}
	if err != nil {
		var refusal *apierror.Refusal
		if errors.As(err, &refusal) {
			return apierror.Write(c, refusal)
		}
		h.logger.Error("Failed to update client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "client not found"))
	}

	auditDetails := map[string]any{
		"client_id":        updatedClient.ClientID,
		"hydra_sync_state": syncStatus.State,
	}
	// Only emit the diff for fields that changed — full before/after snapshots
	// inflate audit rows without aiding forensics on redirect-uri tampering.
	if beforeClient != nil && !stringSlicesEqual(beforeClient.RedirectURIs, updatedClient.RedirectURIs) {
		auditDetails["redirect_uris_before"] = []string(beforeClient.RedirectURIs)
		auditDetails["redirect_uris_after"] = []string(updatedClient.RedirectURIs)
	}
	if beforeClient != nil && !stringSlicesEqual(beforeClient.AllowedOrigins, updatedClient.AllowedOrigins) {
		auditDetails["allowed_origins_before"] = []string(beforeClient.AllowedOrigins)
		auditDetails["allowed_origins_after"] = []string(updatedClient.AllowedOrigins)
	}
	h.logAudit(c, updatedClient.TenantID, audit.ActionClientUpdated, updatedClient.ID.String(), auditDetails)

	if refused := h.respondWithSync(c, syncStatus); refused != nil {
		return refused
	}
	return c.JSON(fiber.Map{
		"message":     "Client updated successfully",
		"client":      updatedClient.ToPublic(),
		"sync_status": syncStatus,
	})
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Delete handles deleting an OAuth client
func (h *ClientHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}
	if err := h.admit(c, id); err != nil {
		return err
	}

	// Snapshot for audit before the row disappears — without this the log
	// entry can't answer "what tenant did the deleted client belong to?"
	beforeClient, _ := h.services.ClientService.GetByID(id)

	syncStatus, err := h.services.ClientService.Delete(id)
	if errors.Is(err, client.ErrSystemClient) {
		return refuseSystemClient(c)
	}
	if err != nil {
		h.logger.Error("Failed to delete client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "client not found"))
	}

	if beforeClient != nil {
		h.logAudit(c, beforeClient.TenantID, audit.ActionClientDeleted, beforeClient.ID.String(), map[string]any{
			"client_id":        beforeClient.ClientID,
			"name":             beforeClient.Name,
			"hydra_sync_state": syncStatus.State,
		})
	}

	if refused := h.respondWithSync(c, syncStatus); refused != nil {
		return refused
	}
	return c.JSON(fiber.Map{
		"message":     "Client deleted successfully",
		"sync_status": syncStatus,
	})
}

// RegenerateSecret handles regenerating client secret
func (h *ClientHandler) RegenerateSecret(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}
	if err := h.admit(c, id); err != nil {
		return err
	}

	// Fetch for tenant_id (required by audit entry) before rotation — the
	// record itself is unchanged by rotation, so reading it first is safe.
	beforeClient, _ := h.services.ClientService.GetByID(id)

	credentials, syncStatus, err := h.services.ClientService.RegenerateSecret(id)
	if errors.Is(err, client.ErrSystemClient) {
		return refuseSystemClient(c)
	}
	if err != nil {
		var refusal *apierror.Refusal
		if errors.As(err, &refusal) {
			return apierror.Write(c, refusal)
		}
		h.logger.Error("Failed to regenerate client secret", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "client not found"))
	}

	// Secret rotation is a security-critical event — use Warning so it
	// surfaces in GetRecentSecurityEvents-style queries alongside lockouts.
	// Reuse ActionClientUpdated with a subresource tag (no dedicated constant
	// exists yet; adding one is scoped to a follow-up change).
	if h.auditService != nil && beforeClient != nil {
		entry := audit.EntryFromFiber(c, beforeClient.TenantID, audit.ActionClientUpdated, "client", id.String())
		entry.Severity = audit.SeverityWarning
		entry.Details["subresource"] = "client_secret"
		entry.Details["event"] = "secret_rotated"
		entry.Details["client_id"] = credentials.ClientID
		entry.Details["hydra_sync_state"] = syncStatus.State
		h.auditService.LogAsync(entry)
	}

	if refused := h.respondWithSync(c, syncStatus); refused != nil {
		return refused
	}
	return c.JSON(fiber.Map{
		"message":     "Client secret regenerated successfully",
		"credentials": credentials,
		"sync_status": syncStatus,
	})
}

// respondWithSync answers 502 when the caller asked for strict sync
// (`?strict_sync=true`) and the upstream sync failed, and returns nil when
// normal best-effort behavior applies — the caller then writes its own success
// body (typically including `sync_status` so callers can detect drift without
// scraping logs).
func (h *ClientHandler) respondWithSync(c *fiber.Ctx, syncStatus client.SyncStatus) *apierror.Refusal {
	if c.Query("strict_sync") == "true" && !syncStatus.OK() {
		return &apierror.Refusal{
			Status:  fiber.StatusBadGateway,
			Code:    "upstream_sync_failed",
			Message: "Upstream OAuth provider sync failed",
			Extensions: fiber.Map{
				"sync_status": syncStatus,
				"hint":        "Authway DB was updated but Hydra rejected the change. Retry, or omit ?strict_sync=true to accept best-effort sync (drift visible in sync_status).",
			},
		}
	}
	return nil
}

// SyncToHydra handles syncing all clients' post_logout_redirect_uris to Hydra
// This is an admin-only endpoint for one-time migration of existing clients
func (h *ClientHandler) SyncToHydra(c *fiber.Ctx) error {
	synced, failed, err := h.services.ClientService.SyncAllClientsToHydra()
	if err != nil {
		h.logger.Error("Failed to sync clients to Hydra", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to sync clients: "+apierror.Message(err, "sync failed"))
	}

	h.logger.Info("Hydra sync completed",
		zap.Int("synced", synced),
		zap.Int("failed", failed))

	return c.JSON(fiber.Map{
		"message": "Hydra sync completed",
		"synced":  synced,
		"failed":  failed,
	})
}

// refuseSystemClient answers a change to an application Authway owns — the
// account screen's client.
func refuseSystemClient(c *fiber.Ctx) error {
	return apierror.RefuseWith(c, fiber.StatusConflict, "system_client", "This application is part of Authway and cannot be changed or deleted.",
		fiber.Map{"hint": "It is the tenant's account screen client; it goes away with the tenant."})
}
