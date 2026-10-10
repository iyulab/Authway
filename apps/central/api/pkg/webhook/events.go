package webhook

import (
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/audit"
)

// EventData is the data of every event except test. It names what the event
// is about and who caused it, and nothing more: a receiver that needs the
// details asks the admin API for them, so no profile data travels in a
// delivery or rests in its recorded payload.
type EventData struct {
	Resource EventRef `json:"resource"`
	Actor    EventRef `json:"actor"`
}

// EventRef points at a resource or an actor. A resource's Type is "user" or
// "client". An actor's Type is what the audit log records as actor_type —
// "user", "api_key", "admin_session", "service_client" or "system" — and its
// ID is empty when the actor has none (the system, or the deployment's key).
type EventRef struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

// auditEvents maps what the audit log records to the event it is sent as. An
// audit action without an entry is not a webhook event.
var auditEvents = map[audit.AuditAction]EventType{
	audit.ActionUserCreated:       EventUserCreated,
	audit.ActionUserUpdated:       EventUserUpdated,
	audit.ActionUserDeleted:       EventUserDeleted,
	audit.ActionUserLogin:         EventUserLogin,
	audit.ActionUserLogout:        EventUserLogout,
	audit.ActionUserPasswordReset: EventUserPasswordChanged,
	audit.ActionUserMFAEnabled:    EventUserMFAEnabled,
	audit.ActionUserMFADisabled:   EventUserMFADisabled,
	audit.ActionClientCreated:     EventClientCreated,
	audit.ActionClientUpdated:     EventClientUpdated,
	audit.ActionClientDeleted:     EventClientDeleted,
}

// FromAudit returns a function that sends the webhook event, if any, for an
// audit entry that has been recorded. Subscribe it to the audit service: the
// audit log is the one place every lifecycle event passes through, so an
// event is sent for exactly what is recorded. Failed operations send nothing.
func FromAudit(webhooks Service, logger *zap.Logger) func(audit.AuditLog) {
	return func(entry audit.AuditLog) {
		event, ok := auditEvents[entry.Action]
		if !ok || !entry.Success || entry.TenantID == uuid.Nil {
			return
		}
		data := EventData{
			Resource: EventRef{Type: entry.ResourceType, ID: entry.ResourceID},
			Actor:    EventRef{Type: entry.ActorType},
		}
		if entry.ActorID != nil {
			data.Actor.ID = entry.ActorID.String()
		}
		if err := webhooks.Trigger(entry.TenantID, event, data); err != nil {
			logger.Error("Failed to send webhook event", zap.String("event", string(event)), zap.Error(err))
		}
	}
}
