package audit

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/tenantscope"

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

func invalid(message string) error {
	return apierror.Reject(fiber.StatusBadRequest, "invalid_request", message)
}

// intQuery reads an optional integer query parameter within [min, max]. A
// value outside the range is refused, never replaced.
func intQuery(c *fiber.Ctx, name string, def, min, max int) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, invalid(fmt.Sprintf("%s must be an integer between %d and %d", name, min, max))
	}
	return n, nil
}

func timeQuery(c *fiber.Ctx, name string) (*time.Time, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, invalid(name + " must be an RFC 3339 time")
	}
	return &t, nil
}

func (h *Handler) failed(c *fiber.Ctx, err error, message string) error {
	h.logger.Error(message, zap.Error(err))
	return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", message)
}

// QueryAuditLogs returns one page of the tenant's audit log, newest first.
// GET /api/v1/audit/logs
func (h *Handler) QueryAuditLogs(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	query := &AuditLogQuery{
		TenantID:     tenantID,
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
	}
	if raw := c.Query("actor_id"); raw != "" {
		actorID, err := uuid.Parse(raw)
		if err != nil {
			return invalid("actor_id must be a user id")
		}
		query.ActorID = &actorID
	}
	if raw := c.Query("action"); raw != "" {
		if !knownAction(raw) {
			return invalid(fmt.Sprintf("unknown action %q; GET /api/v1/audit/actions lists them", raw))
		}
		query.Action = AuditAction(raw)
	}
	if raw := c.Query("severity"); raw != "" {
		if !knownSeverity(raw) {
			return invalid("severity must be one of info, warning, error, critical")
		}
		query.Severity = AuditSeverity(raw)
	}
	switch raw := c.Query("success"); raw {
	case "":
	case "true", "false":
		success := raw == "true"
		query.Success = &success
	default:
		return invalid("success must be true or false")
	}
	if query.StartTime, err = timeQuery(c, "start_time"); err != nil {
		return err
	}
	if query.EndTime, err = timeQuery(c, "end_time"); err != nil {
		return err
	}
	if query.Limit, err = intQuery(c, "limit", 100, 1, 1000); err != nil {
		return err
	}
	if query.Offset, err = intQuery(c, "offset", 0, 0, 1<<31-1); err != nil {
		return err
	}

	logs, total, err := h.service.Query(query)
	if err != nil {
		return h.failed(c, err, "failed to query audit logs")
	}

	return c.JSON(fiber.Map{
		"logs":   logs,
		"total":  total,
		"limit":  query.Limit,
		"offset": query.Offset,
	})
}

// GetAuditLog gets one audit log entry
// GET /api/v1/audit/logs/:id
func (h *Handler) GetAuditLog(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return invalid("invalid audit log ID")
	}

	log, err := h.service.GetByID(id)
	if errors.Is(err, ErrNotFound) {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "audit log not found")
	}
	if err != nil {
		return h.failed(c, err, "failed to get audit log")
	}
	if err := tenantscope.Admit(c, log.TenantID, "audit log not found"); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"log": log})
}

// GetUserActivity returns a user's most recent entries in the tenant.
// GET /api/v1/audit/users/:userId/activity
func (h *Handler) GetUserActivity(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return invalid("invalid user ID")
	}
	limit, err := intQuery(c, "limit", 50, 1, 500)
	if err != nil {
		return err
	}

	logs, err := h.service.GetUserActivity(tenantID, userID, limit)
	if err != nil {
		return h.failed(c, err, "failed to get activity")
	}

	return c.JSON(fiber.Map{"logs": logs})
}

// GetSecurityEvents returns the tenant's warning-and-above entries from the
// last hours.
// GET /api/v1/audit/security
func (h *Handler) GetSecurityEvents(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}
	hours, err := intQuery(c, "hours", 24, 1, 168)
	if err != nil {
		return err
	}

	logs, err := h.service.GetRecentSecurityEvents(tenantID, hours)
	if err != nil {
		return h.failed(c, err, "failed to get security events")
	}

	return c.JSON(fiber.Map{"logs": logs, "hours": hours})
}

// GetAuditSummary counts the tenant's entries over recent windows.
// GET /api/v1/audit/summary
func (h *Handler) GetAuditSummary(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	now := time.Now()
	since := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	count := func(q *AuditLogQuery) (int64, error) {
		q.TenantID, q.Limit = tenantID, 1
		_, total, err := h.service.Query(q)
		return total, err
	}
	failed := false
	day := 24 * time.Hour

	total24h, err1 := count(&AuditLogQuery{StartTime: since(day)})
	total7d, err2 := count(&AuditLogQuery{StartTime: since(7 * day)})
	total30d, err3 := count(&AuditLogQuery{StartTime: since(30 * day)})
	failed24h, err4 := count(&AuditLogQuery{StartTime: since(day), Success: &failed})
	security, err5 := h.service.GetRecentSecurityEvents(tenantID, 24)
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		return h.failed(c, err, "failed to summarize audit logs")
	}

	return c.JSON(fiber.Map{
		"summary": fiber.Map{
			"total_24h":         total24h,
			"total_7d":          total7d,
			"total_30d":         total30d,
			"security_events":   len(security),
			"failed_operations": failed24h,
		},
	})
}

// GetAvailableActions lists the actions and severities an entry can carry.
// GET /api/v1/audit/actions
func (h *Handler) GetAvailableActions(c *fiber.Ctx) error {
	actions := make([]fiber.Map, 0, len(Actions))
	for _, a := range Actions {
		actions = append(actions, fiber.Map{"action": a.Action, "description": a.Description})
	}
	severities := make([]fiber.Map, 0, len(Severities))
	for _, s := range Severities {
		severities = append(severities, fiber.Map{"severity": s.Severity, "description": s.Description})
	}
	return c.JSON(fiber.Map{"actions": actions, "severities": severities})
}

// PurgeOldLogs deletes the tenant's entries older than retention_days.
// DELETE /api/v1/audit/logs/purge
func (h *Handler) PurgeOldLogs(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}
	retentionDays, err := intQuery(c, "retention_days", 90, MinRetentionDays, 36500)
	if err != nil {
		return err
	}

	deleted, err := h.service.PurgeOldLogs(tenantID, retentionDays)
	if err != nil {
		return h.failed(c, err, "failed to purge logs")
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
	audit := app.Group("/audit", adminMiddleware)
	audit.Get("/logs", h.QueryAuditLogs)
	audit.Get("/logs/:id", h.GetAuditLog)
	audit.Get("/users/:userId/activity", h.GetUserActivity)
	audit.Get("/security", h.GetSecurityEvents)
	audit.Get("/summary", h.GetAuditSummary)
	audit.Get("/actions", h.GetAvailableActions)
	audit.Delete("/logs/purge", h.PurgeOldLogs)
}
