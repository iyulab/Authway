package audit

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ActorFromFiber extracts actor identity fields that the adminAuth middleware
// stashes in c.Locals. Missing/unparseable values are returned as zero values
// so audit wiring stays non-fatal even if the middleware changes shape.
func ActorFromFiber(c *fiber.Ctx) (actorType string, actorID *uuid.UUID, actorEmail string, details map[string]interface{}) {
	details = map[string]interface{}{}

	if v, ok := c.Locals("actor_type").(string); ok && v != "" {
		actorType = v
	}
	if v, ok := c.Locals("actor_email").(string); ok && v != "" {
		actorEmail = v
	}
	if v, ok := c.Locals("actor_id").(uuid.UUID); ok && v != uuid.Nil {
		actorID = &v
	}
	if v, ok := c.Locals("actor_key_hint").(string); ok && v != "" {
		details["actor_key_hint"] = v
	}
	if v, ok := c.Locals("auth_method").(string); ok && v != "" {
		details["auth_method"] = v
	}

	return actorType, actorID, actorEmail, details
}

// EntryFromFiber builds an AuditEntry pre-populated with actor identity,
// IP/UA, tenant, action, and resource. Caller merges any action-specific
// fields into Details before calling Service.LogAsync(entry).
func EntryFromFiber(c *fiber.Ctx, tenantID uuid.UUID, action AuditAction, resourceType, resourceID string) *AuditEntry {
	actorType, actorID, actorEmail, details := ActorFromFiber(c)
	return &AuditEntry{
		TenantID:     tenantID,
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		ActorType:    actorType,
		Action:       action,
		Severity:     SeverityInfo,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    c.IP(),
		UserAgent:    c.Get("User-Agent"),
		Details:      details,
		Success:      true,
	}
}
