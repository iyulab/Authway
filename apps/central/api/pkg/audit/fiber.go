package audit

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SupportAccessLocal is the fiber.Ctx local the admin middleware fills, for an
// operator's console account, with the support access it holds open: tenant
// id → support access id (map[uuid.UUID]uuid.UUID). What the operator does in
// such a tenant names the entry in its audit record.
const SupportAccessLocal = "support_access"

// ActorFromFiber extracts actor identity fields that the adminAuth middleware
// stashes in c.Locals. Missing/unparseable values are returned as zero values
// so audit wiring stays non-fatal even if the middleware changes shape.
func ActorFromFiber(c *fiber.Ctx) (actorType string, actorID *uuid.UUID, actorEmail string, details map[string]any) {
	details = map[string]any{}

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
	if open, ok := c.Locals(SupportAccessLocal).(map[uuid.UUID]uuid.UUID); ok {
		if id, ok := open[tenantID]; ok {
			details["support_access_id"] = id.String()
		}
	}
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
