package webhook

import (
	"errors"
	"strconv"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenantscope"

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
func (h *Handler) logAudit(c *fiber.Ctx, tenantID uuid.UUID, action audit.AuditAction, resourceID string, extra map[string]any) {
	if h.auditService == nil {
		return
	}
	entry := audit.EntryFromFiber(c, tenantID, action, "webhook", resourceID)
	for k, v := range extra {
		entry.Details[k] = v
	}
	h.auditService.LogAsync(entry)
}

// refuse answers a service error: a not-found id is 404, a refusal the
// service worded for the caller is 400, anything else 500.
func (h *Handler) refuse(c *fiber.Ctx, err error, fallback string) error {
	if errors.Is(err, ErrNotFound) {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "webhook not found")
	}
	var pub *apierror.Public
	if errors.As(err, &pub) {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", pub.Error())
	}
	h.logger.Error(fallback, zap.Error(err))
	return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", fallback)
}

func webhookID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, apierror.Reject(fiber.StatusBadRequest, "invalid_request", "invalid webhook ID")
	}
	return id, nil
}

// CreateWebhook creates a new webhook
// POST /api/v1/webhooks
func (h *Handler) CreateWebhook(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	var req CreateWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid request body")
	}

	webhook, err := h.service.Create(tenantID, &req)
	if err != nil {
		return h.refuse(c, err, "failed to create webhook")
	}

	h.logAudit(c, tenantID, audit.ActionWebhookCreated, webhook.ID.String(), map[string]any{
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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	webhooks, err := h.service.ListByTenant(tenantID)
	if err != nil {
		return h.refuse(c, err, "failed to list webhooks")
	}

	return c.JSON(fiber.Map{"webhooks": webhooks})
}

// GetWebhook gets a webhook by ID
// GET /api/v1/webhooks/:id
func (h *Handler) GetWebhook(c *fiber.Ctx) error {
	id, err := webhookID(c)
	if err != nil {
		return err
	}

	webhook, err := h.service.GetByID(id)
	if err != nil {
		return h.refuse(c, err, "failed to get webhook")
	}

	return c.JSON(fiber.Map{"webhook": webhook})
}

// UpdateWebhook changes the fields the request names.
// PATCH /api/v1/webhooks/:id
func (h *Handler) UpdateWebhook(c *fiber.Ctx) error {
	id, err := webhookID(c)
	if err != nil {
		return err
	}

	var req UpdateWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid request body")
	}

	webhook, err := h.service.Update(id, &req)
	if err != nil {
		return h.refuse(c, err, "failed to update webhook")
	}

	h.logAudit(c, webhook.TenantID, audit.ActionWebhookUpdated, webhook.ID.String(), map[string]any{
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
	id, err := webhookID(c)
	if err != nil {
		return err
	}

	// Snapshot before deletion so the audit entry can answer which tenant the
	// webhook belonged to after the row is gone.
	before, err := h.service.GetByID(id)
	if err != nil {
		return h.refuse(c, err, "failed to delete webhook")
	}

	if err := h.service.Delete(id); err != nil {
		return h.refuse(c, err, "failed to delete webhook")
	}

	h.logAudit(c, before.TenantID, audit.ActionWebhookDeleted, before.ID.String(), map[string]any{
		"name": before.Name,
		"url":  before.URL,
	})

	return c.JSON(fiber.Map{"message": "webhook deleted successfully"})
}

// GetWebhookDeliveries gets delivery history for a webhook, newest first
// GET /api/v1/webhooks/:id/deliveries
func (h *Handler) GetWebhookDeliveries(c *fiber.Ctx) error {
	id, err := webhookID(c)
	if err != nil {
		return err
	}
	if _, err := h.service.GetByID(id); err != nil {
		return h.refuse(c, err, "failed to get deliveries")
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		l, err := strconv.Atoi(raw)
		if err != nil || l < 1 || l > 100 {
			return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
		}
		limit = l
	}

	deliveries, err := h.service.GetDeliveries(id, limit)
	if err != nil {
		return h.refuse(c, err, "failed to get deliveries")
	}

	return c.JSON(fiber.Map{"deliveries": deliveries})
}

// TestWebhook sends one test event to this webhook and answers with the
// delivery: a receiver that refuses or cannot be reached is a successful call
// whose delivery reports the failure.
// POST /api/v1/webhooks/:id/test
func (h *Handler) TestWebhook(c *fiber.Ctx) error {
	id, err := webhookID(c)
	if err != nil {
		return err
	}

	delivery, err := h.service.Test(id)
	if err != nil {
		return h.refuse(c, err, "failed to test webhook")
	}

	return c.JSON(fiber.Map{"delivery": delivery})
}

// GetAvailableEvents returns the list of available webhook events
// GET /api/v1/webhooks/events
func (h *Handler) GetAvailableEvents(c *fiber.Ctx) error {
	events := make([]fiber.Map, 0, len(Events))
	for _, e := range Events {
		events = append(events, fiber.Map{"type": e.Type, "description": e.Description})
	}
	return c.JSON(fiber.Map{"events": events})
}

// RegisterRoutes registers webhook management routes
// Admin Console uses adminMiddleware which validates admin session and extracts tenant_id
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler, adminMiddleware fiber.Handler) {
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
