package audit

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles audit log HTTP requests
type Handler struct {
	service Service
	logger  *zap.Logger
}

// NewHandler creates a new audit handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// QueryAuditLogs queries audit logs with filters
// GET /api/v1/audit/logs
func (h *Handler) QueryAuditLogs(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	query := &AuditLogQuery{
		TenantID: tenantID,
	}

	// Parse optional filters
	if actorIDStr := c.Query("actor_id"); actorIDStr != "" {
		if actorID, err := uuid.Parse(actorIDStr); err == nil {
			query.ActorID = &actorID
		}
	}

	if action := c.Query("action"); action != "" {
		query.Action = AuditAction(action)
	}

	if resourceType := c.Query("resource_type"); resourceType != "" {
		query.ResourceType = resourceType
	}

	if resourceID := c.Query("resource_id"); resourceID != "" {
		query.ResourceID = resourceID
	}

	if severity := c.Query("severity"); severity != "" {
		query.Severity = AuditSeverity(severity)
	}

	if successStr := c.Query("success"); successStr != "" {
		success := successStr == "true"
		query.Success = &success
	}

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			query.StartTime = &startTime
		}
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			query.EndTime = &endTime
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 1000 {
			query.Limit = limit
		}
	}
	if query.Limit == 0 {
		query.Limit = 100
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			query.Offset = offset
		}
	}

	logs, total, err := h.service.Query(query)
	if err != nil {
		h.logger.Error("Failed to query audit logs", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to query audit logs"})
	}

	return c.JSON(fiber.Map{
		"logs":   logs,
		"total":  total,
		"limit":  query.Limit,
		"offset": query.Offset,
	})
}

// GetAuditLog gets a single audit log by ID
// GET /api/v1/audit/logs/:id
func (h *Handler) GetAuditLog(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid log ID"})
	}

	log, err := h.service.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "audit log not found"})
	}

	return c.JSON(fiber.Map{"log": log})
}

// GetUserActivity gets recent activity for a specific user
// GET /api/v1/audit/users/:userId/activity
func (h *Handler) GetUserActivity(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	userIDStr := c.Params("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	logs, err := h.service.GetUserActivity(tenantID, userID, limit)
	if err != nil {
		h.logger.Error("Failed to get user activity", zap.Error(err), zap.String("user_id", userIDStr))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get activity"})
	}

	return c.JSON(fiber.Map{
		"logs":    logs,
		"count":   len(logs),
		"user_id": userID.String(),
	})
}

// GetSecurityEvents gets recent security-related events
// GET /api/v1/audit/security
func (h *Handler) GetSecurityEvents(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	hours := 24
	if hoursStr := c.Query("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 && h <= 168 {
			hours = h
		}
	}

	logs, err := h.service.GetRecentSecurityEvents(tenantID, hours)
	if err != nil {
		h.logger.Error("Failed to get security events", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get security events"})
	}

	return c.JSON(fiber.Map{
		"logs":  logs,
		"count": len(logs),
		"hours": hours,
	})
}

// GetAuditSummary gets a summary of audit activity
// GET /api/v1/audit/summary
func (h *Handler) GetAuditSummary(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	// Get counts for different time periods
	now := time.Now()
	last24h := now.Add(-24 * time.Hour)
	last7d := now.Add(-7 * 24 * time.Hour)
	last30d := now.Add(-30 * 24 * time.Hour)

	// Get security events count
	securityEvents, err := h.service.GetRecentSecurityEvents(tenantID, 24)
	if err != nil {
		securityEvents = []AuditLog{}
	}

	// Query for total counts
	query24h := &AuditLogQuery{TenantID: tenantID, StartTime: &last24h, Limit: 1}
	_, total24h, _ := h.service.Query(query24h)

	query7d := &AuditLogQuery{TenantID: tenantID, StartTime: &last7d, Limit: 1}
	_, total7d, _ := h.service.Query(query7d)

	query30d := &AuditLogQuery{TenantID: tenantID, StartTime: &last30d, Limit: 1}
	_, total30d, _ := h.service.Query(query30d)

	// Count failed operations in last 24h
	failed := false
	queryFailed := &AuditLogQuery{TenantID: tenantID, StartTime: &last24h, Success: &failed, Limit: 1}
	_, totalFailed, _ := h.service.Query(queryFailed)

	return c.JSON(fiber.Map{
		"summary": fiber.Map{
			"total_24h":          total24h,
			"total_7d":           total7d,
			"total_30d":          total30d,
			"security_events":    len(securityEvents),
			"failed_operations":  totalFailed,
		},
	})
}

// GetAvailableActions returns the list of available audit actions
// GET /api/v1/audit/actions
func (h *Handler) GetAvailableActions(c *fiber.Ctx) error {
	actions := []fiber.Map{
		{"action": "user.created", "description": "User account created"},
		{"action": "user.updated", "description": "User account updated"},
		{"action": "user.deleted", "description": "User account deleted"},
		{"action": "user.login", "description": "User logged in"},
		{"action": "user.login_failed", "description": "Failed login attempt"},
		{"action": "user.logout", "description": "User logged out"},
		{"action": "user.password_changed", "description": "Password changed"},
		{"action": "user.password_reset", "description": "Password reset requested"},
		{"action": "user.locked", "description": "User account locked"},
		{"action": "user.unlocked", "description": "User account unlocked"},
		{"action": "user.mfa_enabled", "description": "MFA enabled"},
		{"action": "user.mfa_disabled", "description": "MFA disabled"},
		{"action": "session.created", "description": "Session created"},
		{"action": "session.revoked", "description": "Session revoked"},
		{"action": "token.issued", "description": "Token issued"},
		{"action": "token.revoked", "description": "Token revoked"},
		{"action": "client.created", "description": "OAuth client created"},
		{"action": "client.updated", "description": "OAuth client updated"},
		{"action": "client.deleted", "description": "OAuth client deleted"},
		{"action": "admin.action", "description": "Administrative action"},
	}

	severities := []fiber.Map{
		{"severity": "info", "description": "Informational events"},
		{"severity": "warning", "description": "Warning events"},
		{"severity": "error", "description": "Error events"},
		{"severity": "critical", "description": "Critical security events"},
	}

	return c.JSON(fiber.Map{
		"actions":    actions,
		"severities": severities,
	})
}

// PurgeOldLogs purges old audit logs (admin only)
// DELETE /api/v1/audit/logs/purge
func (h *Handler) PurgeOldLogs(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	retentionDays := 90 // default
	if daysStr := c.Query("retention_days"); daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days >= 30 {
			retentionDays = days
		}
	}

	deleted, err := h.service.PurgeOldLogs(tenantID, retentionDays)
	if err != nil {
		h.logger.Error("Failed to purge audit logs", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to purge logs"})
	}

	h.logger.Info("Audit logs purged",
		zap.String("tenant_id", tenantID.String()),
		zap.Int64("deleted", deleted),
		zap.Int("retention_days", retentionDays),
	)

	return c.JSON(fiber.Map{
		"message":        "audit logs purged",
		"deleted":        deleted,
		"retention_days": retentionDays,
	})
}

// RegisterRoutes registers audit log routes
// Admin Console uses adminMiddleware which validates admin session and extracts tenant_id
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler, adminMiddleware fiber.Handler) {
	// Admin Console routes - use adminMiddleware only (validates admin session + tenant context)
	audit := app.Group("/audit", adminMiddleware)
	audit.Get("/logs", h.QueryAuditLogs)
	audit.Get("/logs/:id", h.GetAuditLog)
	audit.Get("/users/:userId/activity", h.GetUserActivity)
	audit.Get("/security", h.GetSecurityEvents)
	audit.Get("/summary", h.GetAuditSummary)
	audit.Get("/actions", h.GetAvailableActions)
	audit.Delete("/logs/purge", h.PurgeOldLogs)
}
