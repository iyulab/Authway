package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
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

func newSignIn(hc *hydra.Client, users user.Service, logger *zap.Logger, redisClient redis.UniversalClient, policy SignInPolicy) *signIn {
	return &signIn{
		hydra:           hc,
		users:           users,
		logger:          logger,
		sessions:        policy.Sessions,
		terms:           policy.Terms,
		authentications: NewAuthenticationStore(redisClient),
		pending:         NewMFAChallengeStore(redisClient),
		owing:           NewTermsChallengeStore(redisClient),
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

// accept signs u in to flow by method and returns where the browser goes next.
//
// Every sign-in opens a session at the authorization server. Without one the
// user's next application asks them to sign in again, a logout has nothing to
// end, and applications are not told when the user is signed out elsewhere —
// none of which a user asked for by leaving "keep me signed in" unticked.
// That choice decides only how long the session lasts.
func (s *signIn) accept(flow string, u *user.User, method signInMethod, keepSignedIn bool) (string, error) {
	if !u.Active {
		return "", errAccountSuspended
	}
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
		return "", err
	}
	// The user is signed in whether or not this is recorded.
	if err := s.users.UpdateLastLogin(u.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
	}
	return resp.RedirectTo, nil
}
