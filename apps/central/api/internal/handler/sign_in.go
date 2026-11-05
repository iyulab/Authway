package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"authway/apps/central/api/pkg/ephemeral"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/terms"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
)

// signIn takes a login flow from "this user has proved who they are by one
// means" to its end. Every means of signing in — a password, an emailed link,
// another identity provider — ends here, so a user who has a second factor is
// asked for it whichever way they came in.
type signIn struct {
	hydra           *hydra.Client
	users           user.Service
	logger          *zap.Logger
	authentications *AuthenticationStore
	accepted        *AcceptedFlowStore
	pending         *MFAChallengeStore
	owing           *TermsChallengeStore
	sessions        SessionPolicy
	terms           TermsLedger
}

// SignInPolicy is what a deployment decides about the sign-ins it completes:
// how long the session lasts, and which documents the user must have
// accepted first. A nil Terms asks for none.
type SignInPolicy struct {
	Sessions SessionPolicy
	Terms    TermsLedger
}

func newSignIn(hc *hydra.Client, users user.Service, logger *zap.Logger, store ephemeral.Store, policy SignInPolicy) *signIn {
	return &signIn{
		hydra:           hc,
		users:           users,
		logger:          logger,
		sessions:        policy.Sessions,
		terms:           policy.Terms,
		authentications: NewAuthenticationStore(store),
		accepted:        NewAcceptedFlowStore(store),
		pending:        NewMFAChallengeStore(store),
		owing:           NewTermsChallengeStore(store),
	}
}

// signInStep is what a login flow needs next: exactly one of MFAChallenge,
// TermsChallenge and RedirectTo is set.
type signInStep struct {
	// MFAChallenge is set while the user still owes a second factor. It
	// proves the first one passed — the flow id alone must not open the
	// second step.
	MFAChallenge string
	// TermsChallenge is set while the user still owes the tenant's documents,
	// Terms. It proves every factor passed, as MFAChallenge proves the first.
	TermsChallenge string
	Terms          []terms.Document
	// RedirectTo is set once the flow has been accepted.
	RedirectTo string
	// Replayed is set when the flow had already been accepted for this user
	// and RedirectTo is the answer given then: nothing new was signed in.
	Replayed bool
}

// errAccountSuspended refuses a sign-in to an account an administrator has
// suspended. Every way of signing in passes the steps below, so it is checked
// there: after the user proved who they are (the refusal says nothing to
// anyone who has not), and again where a flow is accepted.
var errAccountSuspended = errors.New("the account is suspended")

// errSecondFactorNotStarted is a failure to park a sign-in for its second
// factor, as opposed to the authorization server refusing the flow.
var errSecondFactorNotStarted = errors.New("could not start the second factor step")

// afterFirstFactor continues flow for a user who has just proved who they are
// by first. A user with a second factor is parked until they give it; anyone
// else is signed in. keepSignedIn is the user's choice to stay signed in on
// this browser (see SessionPolicy).
func (s *signIn) afterFirstFactor(flow string, u *user.User, first signInMethod, keepSignedIn bool) (signInStep, error) {
	if !u.Active {
		return signInStep{}, errAccountSuspended
	}
	if !u.TOTPEnabled {
		return s.proved(flow, u, first, keepSignedIn)
	}
	challenge, err := tokenhash.Generate()
	if err != nil {
		return signInStep{}, fmt.Errorf("%w: %w", errSecondFactorNotStarted, err)
	}
	s.pending.Set(challenge, &pendingMFALogin{
		HydraChallenge: flow,
		UserID:         u.ID,
		First:          first.audit,
		Provider:       first.provider,
		KeepSignedIn:   keepSignedIn,
		CreatedAt:      time.Now(),
	})
	return signInStep{MFAChallenge: challenge}, nil
}

// replay answers a submit of a flow that was already accepted, for owner and
// to: the same answer for the user it was accepted for, none for anyone else.
func replay(owner uuid.UUID, to string, u *user.User) (string, bool, error) {
	if owner != u.ID {
		return "", false, fmt.Errorf("%w: the flow was accepted for another user", hydra.ErrFlowExpired)
	}
	return to, true, nil
}

// accept signs u in to flow by method and returns where the browser goes next.
//
// Every sign-in opens a session at the authorization server. Without one the
// user's next application asks them to sign in again, a logout has nothing to
// end, and applications are not told when the user is signed out elsewhere —
// none of which a user asked for by leaving "keep me signed in" unticked.
// That choice decides only how long the session lasts.
//
// A flow is signed in to once. Asked again — the screen is still up while the
// browser follows the redirect — the user it was accepted for gets the same
// redirect, replayed set, and nothing is signed in a second time; anyone else
// is told the flow is over.
func (s *signIn) accept(flow string, u *user.User, method signInMethod, keepSignedIn bool) (redirectTo string, replayed bool, err error) {
	if !u.Active {
		return "", false, errAccountSuspended
	}
	if owner, to, ok := s.accepted.Recall(flow); ok {
		return replay(owner, to, u)
	}
	// Two submits that overlap are one sign-in: the second waits for the
	// first's answer instead of having the flow accepted again.
	if !s.accepted.Begin(flow) {
		if owner, to, ok := s.accepted.Await(flow); ok {
			return replay(owner, to, u)
		}
		// The other submit did not get the flow accepted; this one tries.
	}
	defer s.accepted.End(flow)
	resp, err := acceptAuthenticatedLogin(s.hydra, s.authentications, flow, &hydra.AcceptLoginRequest{
		Subject:     u.ID.String(),
		Remember:    true,
		RememberFor: int(s.sessions.lifetime(u.TenantID, keepSignedIn).Seconds()),
		AMR:         method.amr,
		Context: map[string]any{
			"email":     u.Email,
			"name":      u.Name,
			"tenant_id": u.TenantID.String(),
		},
	})
	if err != nil {
		return "", false, err
	}
	s.accepted.Remember(flow, u.ID, resp.RedirectTo)
	// The user is signed in whether or not this is recorded.
	if err := s.users.UpdateLastLogin(u.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
	}
	return resp.RedirectTo, false, nil
}
