package social

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type stubGate struct {
	invited bool
	err     error
	calls   int
}

func (s *stubGate) MayProvision(uuid.UUID, string) (bool, error) {
	s.calls++
	return s.invited, s.err
}

// The gate governs whether social sign-in may *create* an account. Every
// answer other than a clean "yes, invited" must deny, because the failure mode
// being guarded against is silently reopening self-registration.
func TestMayProvision(t *testing.T) {
	tenantID := uuid.New()
	logger := zap.NewNop()

	cases := []struct {
		name string
		gate InvitationGate
		want bool
	}{
		{"invited address is allowed", &stubGate{invited: true}, true},
		{"uninvited address is denied", &stubGate{invited: false}, false},
		{"lookup error denies (fails closed)", &stubGate{err: errors.New("db down")}, false},
		{"unwired gate denies (fails closed)", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mayProvision(tc.gate, logger, tenantID, "someone@example.com"); got != tc.want {
				t.Errorf("mayProvision = %v, want %v", got, tc.want)
			}
		})
	}
}

// The tenant is part of the question: an invitation into one organisation must
// not admit the same address to another. The gate receives both, so this pins
// that the caller actually consults it rather than short-circuiting.
func TestMayProvision_ConsultsGate(t *testing.T) {
	gate := &stubGate{invited: true}
	if !mayProvision(gate, zap.NewNop(), uuid.New(), "someone@example.com") {
		t.Fatal("expected allow")
	}
	if gate.calls != 1 {
		t.Errorf("gate consulted %d times, want exactly 1", gate.calls)
	}
}
