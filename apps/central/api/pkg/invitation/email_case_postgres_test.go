package invitation

import (
	"errors"
	"testing"

	"authway/apps/central/api/pkg/tenant"
	"github.com/google/uuid"
)

// An address that differs only in case is the same address: the invitation is
// stored in lower case, the gate finds it however the address is written, and
// a second invitation for the same address in another case is refused.
func TestInvitation_AddressIsOneWhateverTheCase(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, _ := newTestService(t, db)
	tn := tenantWith(t, ts, tenant.TenantSettings{})
	gate := NewGate(db)

	local := "inv-" + uuid.NewString()[:8]
	inv, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: " " + local + "@Example.COM"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID) })

	if want := local + "@example.com"; inv.Email != want {
		t.Fatalf("stored address = %q, want %q", inv.Email, want)
	}
	for _, written := range []string{local + "@example.com", local + "@EXAMPLE.com", " " + local + "@Example.Com "} {
		if ok, err := gate.HasValidInvitation(tn.ID, written); err != nil || !ok {
			t.Errorf("HasValidInvitation(%q) = %v, %v; want true", written, ok, err)
		}
		if ok, err := gate.MayProvision(tn.ID, written); err != nil || !ok {
			t.Errorf("MayProvision(%q) = %v, %v; want true", written, ok, err)
		}
	}

	if _, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: local + "@EXAMPLE.COM"}); !errors.Is(err, ErrAlreadyInvited) {
		t.Errorf("a second invitation for the same address in another case: got %v, want ErrAlreadyInvited", err)
	}
}
