package social

import (
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// InvitationGate reports whether a social sign-in may create a new account
// for (tenantID, email) — invited, or the tenant allows open signup.
//
// Onboarding is invitation-only by default (decision D-a/B), and social login
// was the last path that ignored it: a first-time OAuth sign-in provisioned
// an account for whoever completed the flow, so anyone with a Google account
// could join a tenant they were never invited to. A tenant may opt into open
// signup (tenant.SignupModeOpen); every other tenant keeps the invite-only
// default. Just-in-time provisioning is normal for a general-purpose IdP,
// but Authway defaults to invitation-only membership.
//
// Satisfied by invitation.Service and invitation.Gate.
type InvitationGate interface {
	MayProvision(tenantID uuid.UUID, email string) (bool, error)
}

// mayProvision reports whether a social sign-in may create a new account. It
// fails closed on a missing gate or a lookup error: the safe answer to "is this
// person allowed in?" is no, and a wiring mistake must not silently reopen
// self-registration.
//
// This governs account *creation* only. Signing in to an account that already
// exists never consults it, so existing members are unaffected.
func mayProvision(gate InvitationGate, logger *zap.Logger, tenantID uuid.UUID, email string) bool {
	if gate == nil {
		logger.Error("Invitation gate not wired; denying social provisioning")
		return false
	}
	allowed, err := gate.MayProvision(tenantID, email)
	if err != nil {
		logger.Error("Provisioning check failed; denying social provisioning", zap.Error(err))
		return false
	}
	return allowed
}

// ErrNotInvited is returned when a social sign-in would have to create an
// account for an address nobody invited. The wording is deliberately the same
// across providers so the UI can present one message.
const ErrNotInvited = "no account for this address; ask an administrator for an invitation"
