package webhook

import (
	"strconv"

	"authway/apps/central/api/pkg/audit"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles webhook management HTTP requests
type Handler struct {
	service      Service
	logger       *zap.Logger
	auditService audit.Service
}

// NewHandler creates a new webhook handler
func NewHandler(service Service, logger *zap.Logger, auditService audit.Service) *Handler {
	return &Handler{
		service:      service,
		logger:       logger,
		auditService: auditService,
	}
}

// logAudit records an audit entry for a webhook admin write path. Mirrors the
// client/tenant/user handler pattern — best-effort, nil auditService tolerated.
func (h *Handler) logAudit(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, resourceID string, extra map[string]interface{}) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, "webhook", resourceID)
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// CreateWebhook creates a new webhook
// POST /api/v1/webhooks
func (h *Handler) CreateWebhook(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	var req CreateWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "url is required"})
	}
	if len(req.Events) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "at least one event is required"})
	}

	webhook, err := h.service.Create(tenantID, &req)
	if err != nil {
		h.logger.Warn("Failed to create webhook", zap.Error(err), zap.String("name", req.Name))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Webhook created",
		zap.String("webhook_id", webhook.ID.String()),
		zap.String("name", webhook.Name),
		zap.String("tenant_id", tenantID.String()),
	)

	h.logAudit(c, tenantID, audit.ActionWebhookCreated, webhook.ID.String(), map[string]interface{}{
		"name":   webhook.Name,
		"url":    webhook.URL,
		"events": req.Events,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"webhook": webhook,
		"message": "webhook created successfully",
	})
}

// ListWebhooks lists all webhooks for the tenant
// GET /api/v1/webhooks
func (h *Handler) ListWebhooks(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	webhooks, err := h.service.ListByTenant(tenantID)
	if err != nil {
		h.logger.Error("Failed to list webhooks", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list webhooks"})
	}

	return c.JSON(fiber.Map{
		"webhooks": webhooks,
		"count":    len(webhooks),
	})
}

// GetWebhook gets a webhook by ID
// GET /api/v1/webhooks/:id
func (h *Handler) GetWebhook(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid webhook ID"})
	}

	webhook, err := h.service.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "webhook not found"})
	}

	return c.JSON(fiber.Map{"webhook": webhook})
}

// UpdateWebhook updates a webhook
// PATCH /api/v1/webhooks/:id
func (h *Handler) UpdateWebhook(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid webhook ID"})
	}

	var req UpdateWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	webhook, err := h.service.Update(id, &req)
	if err != nil {
		h.logger.Warn("Failed to update webhook", zap.Error(err), zap.String("webhook_id", idStr))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Webhook updated", zap.String("webhook_id", webhook.ID.String()))

	h.logAudit(c, webhook.TenantID, audit.ActionWebhookUpdated, webhook.ID.String(), map[string]interface{}{
		"name": webhook.Name,
	})

	return c.JSON(fiber.Map{
		"webhook": webhook,
		"message": "webhook updated successfully",
	})
}

// DeleteWebhook deletes a webhook
// DELETE /api/v1/webhooks/:id
func (h *Handler) DeleteWebhook(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid webhook ID"})
	}

	// Snapshot before deletion so the audit entry can answer which tenant the
	// webhook belonged to after the row is gone.
	before, _ := h.service.GetByID(id)

	if err := h.service.Delete(id); err != nil {
		h.logger.Warn("Failed to delete webhook", zap.Error(err), zap.String("webhook_id", idStr))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	h.logger.Info("Webhook deleted", zap.String("webhook_id", idStr))

	if before != nil {
		h.logAudit(c, before.TenantID, audit.ActionWebhookDeleted, before.ID.String(), map[string]interface{}{
			"name": before.Name,
			"url":  before.URL,
		})
	}

	return c.JSON(fiber.Map{"message": "webhook deleted successfully"})
}

// GetWebhookDeliveries gets delivery history for a webhook
// GET /api/v1/webhooks/:id/deliveries
func (h *Handler) GetWebhookDeliveries(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid webhook ID"})
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	deliveries, err := h.service.GetDeliveries(id, limit)
	if err != nil {
		h.logger.Error("Failed to get webhook deliveries", zap.Error(err), zap.String("webhook_id", idStr))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get deliveries"})
	}

	return c.JSON(fiber.Map{
		"deliveries": deliveries,
		"count":      len(deliveries),
	})
}

// TestWebhook triggers a test event for a webhook
// POST /api/v1/webhooks/:id/test
func (h *Handler) TestWebhook(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid webhook ID"})
	}

	webhook, err := h.service.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "webhook not found"})
	}

	// Trigger a test event
	testData := map[string]interface{}{
		"test":    true,
		"message": "This is a test webhook delivery",
	}

	if err := h.service.Trigger(webhook.TenantID, EventTypeTest, testData); err != nil {
		h.logger.Warn("Failed to trigger test webhook", zap.Error(err), zap.String("webhook_id", idStr))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to trigger test"})
	}

	return c.JSON(fiber.Map{"message": "test webhook triggered"})
}

// GetAvailableEvents returns the list of available webhook events
// GET /api/v1/webhooks/events
func (h *Handler) GetAvailableEvents(c *fiber.Ctx) error {
	events := []fiber.Map{
		{"type": "user.created", "description": "Triggered when a new user is created"},
		{"type": "user.updated", "description": "Triggered when a user is updated"},
		{"type": "user.deleted", "description": "Triggered when a user is deleted"},
		{"type": "user.login", "description": "Triggered when a user logs in"},
		{"type": "user.logout", "description": "Triggered when a user logs out"},
		{"type": "user.password_changed", "description": "Triggered when a user changes their password"},
		{"type": "user.mfa_enabled", "description": "Triggered when MFA is enabled"},
		{"type": "user.mfa_disabled", "description": "Triggered when MFA is disabled"},
		{"type": "session.created", "description": "Triggered when a new session is created"},
		{"type": "session.revoked", "description": "Triggered when a session is revoked"},
		{"type": "client.created", "description": "Triggered when a new OAuth client is created"},
		{"type": "client.updated", "description": "Triggered when an OAuth client is updated"},
		{"type": "client.deleted", "description": "Triggered when an OAuth client is deleted"},
		{"type": "test", "description": "Test event for webhook validation"},
		{"type": "*", "description": "Subscribe to all events"},
	}

	return c.JSON(fiber.Map{"events": events})
}

// RegisterRoutes registers webhook management routes
// Admin Console uses adminMiddleware which validates admin session and extracts tenant_id
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler, adminMiddleware fiber.Handler) {
	// Admin Console routes - use adminMiddleware only (validates admin session + tenant context)
	webhooks := app.Group("/webhooks", adminMiddleware)
	webhooks.Get("/events", h.GetAvailableEvents)
	webhooks.Post("/", h.CreateWebhook)
	webhooks.Get("/", h.ListWebhooks)
	webhooks.Get("/:id", h.GetWebhook)
	webhooks.Patch("/:id", h.UpdateWebhook)
	webhooks.Delete("/:id", h.DeleteWebhook)
	webhooks.Get("/:id/deliveries", h.GetWebhookDeliveries)
	webhooks.Post("/:id/test", h.TestWebhook)
}
