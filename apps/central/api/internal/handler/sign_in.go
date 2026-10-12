package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
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
}

func newSignIn(hc *hydra.Client, users user.Service, logger *zap.Logger, redisClient *redis.Client) *signIn {
	return &signIn{
		hydra:           hc,
		users:           users,
		logger:          logger,
		authentications: NewAuthenticationStore(redisClient),
		pending:         NewMFAChallengeStore(redisClient),
	}
}

// signInStep is what a login flow needs next: exactly one field is set.
type signInStep struct {
	// MFAChallenge is set while the user still owes a second factor. It
	// proves the first one passed — the flow id alone must not open the
	// second step.
	MFAChallenge string
	// RedirectTo is set once the flow has been accepted.
	RedirectTo string
}

// errSecondFactorNotStarted is a failure to park a sign-in for its second
// factor, as opposed to the authorization server refusing the flow.
var errSecondFactorNotStarted = errors.New("could not start the second factor step")

// afterFirstFactor continues flow for a user who has just proved who they are
// by first. A user with a second factor is parked until they give it; anyone
// else is signed in.
func (s *signIn) afterFirstFactor(flow string, u *user.User, first signInMethod, remember bool, rememberFor int) (signInStep, error) {
	if !u.TOTPEnabled {
		redirectTo, err := s.accept(flow, u, first, remember, rememberFor)
		return signInStep{RedirectTo: redirectTo}, err
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
		Remember:       remember,
		RememberFor:    rememberFor,
		CreatedAt:      time.Now(),
	})
	return signInStep{MFAChallenge: challenge}, nil
}

// accept signs u in to flow by method and returns where the browser goes next.
func (s *signIn) accept(flow string, u *user.User, method signInMethod, remember bool, rememberFor int) (string, error) {
	resp, err := acceptAuthenticatedLogin(s.hydra, s.authentications, flow, &hydra.AcceptLoginRequest{
		Subject:     u.ID.String(),
		Remember:    remember,
		RememberFor: rememberFor,
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
