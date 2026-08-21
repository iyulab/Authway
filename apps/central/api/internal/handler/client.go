package handler

import (
	"strconv"

	"authway/apps/central/api/internal/config"
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
	config       *config.Config
	auditService audit.Service
}

func NewClientHandler(services *service.Services, logger *zap.Logger, cfg *config.Config, auditService audit.Service) *ClientHandler {
	return &ClientHandler{
		services:     services,
		logger:       logger,
		validator:    validator.New(),
		config:       cfg,
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
	tenantIDStr := c.Query("tenant_id", "")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	var clients []*client.Client
	var total int64

	// If tenant_id is provided, filter by tenant
	if tenantIDStr != "" {
		tenantID, err := uuid.Parse(tenantIDStr)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid tenant_id format")
		}

		clients, total, err = h.services.ClientService.GetByTenant(tenantID, limit, offset)
		if err != nil {
			h.logger.Error("Failed to list clients by tenant", zap.Error(err), zap.String("tenant_id", tenantIDStr))
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
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	newClient, credentials, err := h.services.ClientService.Create(&req)
	if err != nil {
		// Surface client-config violations as 400 with a stable code/hint payload
		// so SDKs and the Admin Console can render targeted UX.
		if cerr, ok := err.(*client.ConfigError); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   cerr.Message,
				"code":    cerr.Code,
				"field":   cerr.Field,
				"hint":    cerr.Hint,
			})
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
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	tenantID, _ := c.Locals("tenant_id").(string)
	req := scoped.ToCreateClientRequest(tenantID)

	newClient, credentials, err := h.services.ClientService.Create(req)
	if err != nil {
		if cerr, ok := err.(*client.ConfigError); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": cerr.Message,
				"code":  cerr.Code,
				"field": cerr.Field,
				"hint":  cerr.Hint,
			})
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

// Get handles getting a specific OAuth client by ID
func (h *ClientHandler) Get(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
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

// GetByClientID handles getting a specific OAuth client by client_id (for internal use)
func (h *ClientHandler) GetByClientID(c *fiber.Ctx) error {
	clientID := c.Params("client_id")
	if clientID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "client_id is required")
	}

	foundClient, err := h.services.ClientService.GetByClientID(clientID)
	if err != nil {
		h.logger.Error("Failed to get client by client_id", zap.Error(err), zap.String("client_id", clientID))
		return fiber.NewError(fiber.StatusNotFound, "Client not found")
	}

	// Return full client including logout policy fields (for internal Auth API use)
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

	var req client.UpdateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	// Capture before-state for audit diff. Missing record surfaces as the
	// same 404 the Update call would raise, so we only take the snapshot on
	// success and let Update own the canonical error path.
	beforeClient, _ := h.services.ClientService.GetByID(id)

	updatedClient, syncStatus, err := h.services.ClientService.Update(id, &req)
	if err != nil {
		if cerr, ok := err.(*client.ConfigError); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": cerr.Message,
				"code":  cerr.Code,
				"field": cerr.Field,
				"hint":  cerr.Hint,
			})
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

	if status, body := h.respondWithSync(c, syncStatus); status != 0 {
		return c.Status(status).JSON(body)
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

	// Snapshot for audit before the row disappears — without this the log
	// entry can't answer "what tenant did the deleted client belong to?"
	beforeClient, _ := h.services.ClientService.GetByID(id)

	syncStatus, err := h.services.ClientService.Delete(id)
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

	if status, body := h.respondWithSync(c, syncStatus); status != 0 {
		return c.Status(status).JSON(body)
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

	// Fetch for tenant_id (required by audit entry) before rotation — the
	// record itself is unchanged by rotation, so reading it first is safe.
	beforeClient, _ := h.services.ClientService.GetByID(id)

	credentials, syncStatus, err := h.services.ClientService.RegenerateSecret(id)
	if err != nil {
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

	if status, body := h.respondWithSync(c, syncStatus); status != 0 {
		return c.Status(status).JSON(body)
	}
	return c.JSON(fiber.Map{
		"message":     "Client secret regenerated successfully",
		"credentials": credentials,
		"sync_status": syncStatus,
	})
}

// respondWithSync returns (status, body) when the caller asked for strict sync
// (`?strict_sync=true`) and the upstream sync failed — caller should write that
// 502 response. Returns (0, nil) when normal best-effort behavior applies and
// the caller should write its own success body (typically including
// `sync_status` so callers can detect drift without scraping logs).
func (h *ClientHandler) respondWithSync(c *fiber.Ctx, syncStatus client.SyncStatus) (int, fiber.Map) {
	if c.Query("strict_sync") == "true" && !syncStatus.OK() {
		return fiber.StatusBadGateway, fiber.Map{
			"error":       "Upstream OAuth provider sync failed",
			"sync_status": syncStatus,
			"hint":        "Authway DB was updated but Hydra rejected the change. Retry, or omit ?strict_sync=true to accept best-effort sync (drift visible in sync_status).",
		}
	}
	return 0, nil
}

// UpdateGoogleOAuth handles updating Google OAuth configuration for a client
func (h *ClientHandler) UpdateGoogleOAuth(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	type GoogleOAuthRequest struct {
		GoogleClientID     string `json:"google_client_id" validate:"required"`
		GoogleClientSecret string `json:"google_client_secret" validate:"required"`
		GoogleRedirectURI  string `json:"google_redirect_uri" validate:"required,url"`
	}

	var req GoogleOAuthRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	// Update client with Google OAuth settings
	updateReq := &client.UpdateClientRequest{
		GoogleOAuthEnabled: &[]bool{true}[0], // Pointer to true
		GoogleClientID:     &req.GoogleClientID,
		GoogleClientSecret: &req.GoogleClientSecret,
		GoogleRedirectURI:  &req.GoogleRedirectURI,
	}

	updatedClient, _, err := h.services.ClientService.Update(id, updateReq)
	if err != nil {
		h.logger.Error("Failed to update client Google OAuth", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "client not found"))
	}

	h.logAudit(c, updatedClient.TenantID, audit.ActionClientUpdated, updatedClient.ID.String(), map[string]any{
		"subresource":      "google_oauth",
		"event":            "google_oauth_configured",
		"client_id":        updatedClient.ClientID,
		"google_client_id": req.GoogleClientID,
	})

	h.logger.Info("Client Google OAuth updated successfully",
		zap.String("id", idStr),
		zap.String("google_client_id", req.GoogleClientID))

	return c.JSON(fiber.Map{
		"message": "Google OAuth configuration updated successfully",
		"client":  updatedClient.ToPublic(),
	})
}

// DisableGoogleOAuth handles disabling Google OAuth for a client
func (h *ClientHandler) DisableGoogleOAuth(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	// Update client to disable Google OAuth
	updateReq := &client.UpdateClientRequest{
		GoogleOAuthEnabled: &[]bool{false}[0], // Pointer to false
		GoogleClientID:     nil,
		GoogleClientSecret: nil,
		GoogleRedirectURI:  nil,
	}

	updatedClient, _, err := h.services.ClientService.Update(id, updateReq)
	if err != nil {
		h.logger.Error("Failed to disable client Google OAuth", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "client not found"))
	}

	h.logAudit(c, updatedClient.TenantID, audit.ActionClientUpdated, updatedClient.ID.String(), map[string]any{
		"subresource": "google_oauth",
		"event":       "google_oauth_disabled",
		"client_id":   updatedClient.ClientID,
	})

	h.logger.Info("Client Google OAuth disabled successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message": "Google OAuth configuration disabled successfully",
		"client":  updatedClient.ToPublic(),
	})
}

// GetGoogleOAuthStatus handles getting Google OAuth status for a client
func (h *ClientHandler) GetGoogleOAuthStatus(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	foundClient, err := h.services.ClientService.GetByID(id)
	if err != nil {
		h.logger.Error("Failed to get client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, "Client not found")
	}

	status := "disabled"
	oauthType := "central"

	if foundClient.GoogleOAuthEnabled && foundClient.GoogleClientID != nil && foundClient.GoogleClientSecret != nil {
		status = "enabled"
		oauthType = "client_specific"
	}

	response := fiber.Map{
		"client_id":           foundClient.ClientID,
		"google_oauth_status": status,
		"oauth_type":          oauthType,
		"google_redirect_uri": foundClient.GoogleRedirectURI,
	}

	// Include Google Client ID (but not secret) if enabled
	if status == "enabled" {
		response["google_client_id"] = foundClient.GoogleClientID
	}

	return c.JSON(response)
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

// GetPublicConfig handles getting public OAuth configuration for a client by client_id
// This is a PUBLIC endpoint that doesn't require authentication
// Used by SDK to auto-configure OAuth settings
func (h *ClientHandler) GetPublicConfig(c *fiber.Ctx) error {
	clientID := c.Params("client_id")
	if clientID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "client_id is required")
	}

	// Get client by client_id (not UUID)
	foundClient, err := h.services.ClientService.GetByClientID(clientID)
	if err != nil {
		h.logger.Error("Failed to get client config", zap.Error(err), zap.String("client_id", clientID))
		return fiber.NewError(fiber.StatusNotFound, "Client not found")
	}

	// Return public configuration (no secrets)
	return c.JSON(fiber.Map{
		"client_id":     foundClient.ClientID,
		"oauth_url":     h.config.Hydra.PublicURL,
		"redirect_uris": foundClient.RedirectURIs,
		"scopes":        []string{"openid", "profile", "email"}, // Default scopes
		"tenant_id":     foundClient.TenantID,
	})
}
