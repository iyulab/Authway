package webhook

import (
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/audit"
)

type triggered struct {
	tenant uuid.UUID
	event  EventType
	data   any
}

// recordingService records what would be sent instead of sending it.
type recordingService struct {
	Service
	sent []triggered
}

func (r *recordingService) Trigger(tenantID uuid.UUID, eventType EventType, data any) error {
	r.sent = append(r.sent, triggered{tenantID, eventType, data})
	return nil
}

func TestFromAudit_SendsTheEventForARecordedEntry(t *testing.T) {
	rec := &recordingService{}
	emit := FromAudit(rec, zap.NewNop())
	tenant, actor, target := uuid.New(), uuid.New(), uuid.New()

	emit(audit.AuditLog{
		TenantID: tenant, Action: audit.ActionUserDeleted, Success: true,
		ResourceType: "user", ResourceID: target.String(), ActorType: "admin", ActorID: &actor,
	})

	if len(rec.sent) != 1 {
		t.Fatalf("sent %d events, want 1", len(rec.sent))
	}
	got := rec.sent[0]
	want := EventData{Resource: EventRef{Type: "user", ID: target.String()}, Actor: EventRef{Type: "admin", ID: actor.String()}}
	if got.tenant != tenant || got.event != EventUserDeleted || got.data != want {
		t.Errorf("sent %+v, want tenant %s event %s data %+v", got, tenant, EventUserDeleted, want)
	}
}

func TestFromAudit_SendsNothingForWhatIsNotAnEvent(t *testing.T) {
	rec := &recordingService{}
	emit := FromAudit(rec, zap.NewNop())
	tenant := uuid.New()

	for name, entry := range map[string]audit.AuditLog{
		"a failed operation":             {TenantID: tenant, Action: audit.ActionUserLogin, Success: false},
		"an action that is not an event": {TenantID: tenant, Action: audit.ActionUserLoginFailed, Success: true},
		"an entry outside any tenant":    {Action: audit.ActionUserLogin, Success: true},
	} {
		emit(entry)
		if len(rec.sent) != 0 {
			t.Fatalf("%s sent %+v", name, rec.sent)
		}
	}
}

// Every event a webhook can subscribe to must have something that sends it,
// and everything sent must be on the list — an event offered but never sent
// is a feature that only looks like it works.
func TestEveryOfferedEventIsSent(t *testing.T) {
	sent := map[EventType]bool{EventTypeTest: true, EventAll: true}
	for _, event := range auditEvents {
		sent[event] = true
		if !knownEvent(string(event)) {
			t.Errorf("%s is sent but not offered", event)
		}
	}
	for _, offered := range Events {
		if !sent[offered.Type] {
			t.Errorf("%s is offered but nothing sends it", offered.Type)
		}
	}
}
