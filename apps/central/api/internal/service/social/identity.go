package social

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/user"
)

// Identity is what a sign-in provider asserted about the person who just
// signed in with it.
type Identity struct {
	// Provider is "google", "github", "microsoft" or "apple".
	Provider string
	// Subject is the provider's own identifier for the account. It does not
	// change when the person changes their address.
	Subject string
	Email   string
	// EmailVerified is true only when the provider states that the person
	// controls Email. An address the provider merely stores — one an
	// administrator of the provider-side directory can set freely — is not
	// verified.
	EmailVerified bool
	Name          string
	Picture       string
}

// ErrEmailNotVerified refuses a first sign-in with a provider account whose
// address the provider has not verified. An account is found by address only
// once, to link it; an unverified address would let whoever controls the
// provider-side account sign in as the owner of the address.
var ErrEmailNotVerified = errors.New("the sign-in provider has not verified this email address")

// ErrLinkedToAnother refuses a sign-in whose address belongs to an account
// that already signs in with a different account at the same provider.
var ErrLinkedToAnother = errors.New("this email address already signs in with a different account at this provider")

// accounts is the part of the user service that resolving an identity needs.
type accounts interface {
	GetByProviderSubject(tenantID uuid.UUID, provider, subject string) (*user.User, error)
	GetByEmailAndTenant(tenantID uuid.UUID, email string) (*user.User, error)
	Create(tenantID uuid.UUID, req *user.CreateUserRequest) (*user.User, error)
	LinkProvider(userID uuid.UUID, provider, subject string) error
	Update(id uuid.UUID, req *user.UpdateUserRequest) (*user.User, error)
}

// resolveAccount returns the account in tenantID that the identity signs in
// to, creating or linking one where the rules allow:
//
//  1. An account already linked to (provider, subject) is that person's
//     account, whatever address the provider reports now.
//  2. Otherwise the address decides, once — and only an address the provider
//     has verified. An existing account with that address is linked; it must
//     not already be linked to a different account at the same provider.
//  3. With no such account, one is created if the tenant admits the address
//     (invited, or open signup), and linked.
//
// After the first sign-in the link identifies the account, so a later change
// of address at the provider neither loses the account nor reaches another.
func resolveAccount(users accounts, gate InvitationGate, logger *zap.Logger, tenantID uuid.UUID, id Identity) (*user.User, error) {
	if id.Subject == "" {
		return nil, fmt.Errorf("%s did not identify the account", id.Provider)
	}

	linked, err := users.GetByProviderSubject(tenantID, id.Provider, id.Subject)
	switch {
	case err == nil:
		return linked, nil
	case !errors.Is(err, user.ErrUserNotFound):
		return nil, fmt.Errorf("failed to look up the linked account: %w", err)
	}

	if id.Email == "" {
		return nil, fmt.Errorf("email not available from the %s account", id.Provider)
	}
	if !id.EmailVerified {
		logger.Warn("Social sign-in denied: the provider has not verified the address",
			zap.String("provider", id.Provider), zap.String("email", id.Email))
		return nil, ErrEmailNotVerified
	}

	account, err := users.GetByEmailAndTenant(tenantID, id.Email)
	switch {
	case err == nil:
		if other := linkedSubject(account, id.Provider); other != "" && other != id.Subject {
			logger.Warn("Social sign-in denied: the address is linked to another provider account",
				zap.String("provider", id.Provider), zap.String("user_id", account.ID.String()))
			return nil, ErrLinkedToAnother
		}
	case errors.Is(err, user.ErrUserNotFound):
		// Onboarding is invitation-only by default: a first sign-in may
		// create an account only for an address the tenant admits.
		if !mayProvision(gate, logger, tenantID, id.Email) {
			logger.Warn("Social sign-in denied for uninvited address", zap.String("email", id.Email))
			return nil, ErrNotInvited
		}
		account, err = users.Create(tenantID, &user.CreateUserRequest{Email: id.Email, Name: id.Name})
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
		logger.Info("Created an account from a social sign-in",
			zap.String("provider", id.Provider), zap.String("user_id", account.ID.String()), zap.String("tenant_id", tenantID.String()))
	default:
		return nil, fmt.Errorf("failed to look up the account: %w", err)
	}

	err = users.LinkProvider(account.ID, id.Provider, id.Subject)
	if errors.Is(err, user.ErrProviderSubjectTaken) {
		// Another sign-in with the same provider account linked it between
		// the lookup above and here. Rule 1 holds: the account it is linked
		// to is this person's account.
		logger.Warn("The provider account was linked by a concurrent sign-in; using that account",
			zap.String("provider", id.Provider), zap.String("tenant_id", tenantID.String()))
		linked, err := users.GetByProviderSubject(tenantID, id.Provider, id.Subject)
		if err != nil {
			return nil, fmt.Errorf("failed to look up the linked account: %w", err)
		}
		return linked, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to link the account: %w", err)
	}
	account.EmailVerified = true
	setLinkedSubject(account, id.Provider, id.Subject)

	if id.Picture != "" && account.AvatarURL == nil {
		if _, err := users.Update(account.ID, &user.UpdateUserRequest{AvatarURL: &id.Picture}); err != nil {
			logger.Warn("Failed to store the provider's picture", zap.Error(err))
		}
	}
	return account, nil
}

// linkedSubject returns the provider account u already signs in with, or "".
func linkedSubject(u *user.User, provider string) string {
	var subject *string
	switch provider {
	case "google":
		subject = u.GoogleID
	case "github":
		subject = u.GithubID
	case "microsoft":
		subject = u.MicrosoftID
	case "apple":
		subject = u.AppleID
	}
	if subject == nil {
		return ""
	}
	return *subject
}

func setLinkedSubject(u *user.User, provider, subject string) {
	switch provider {
	case "google":
		u.GoogleID = &subject
	case "github":
		u.GithubID = &subject
	case "microsoft":
		u.MicrosoftID = &subject
	case "apple":
		u.AppleID = &subject
	}
}

// nonEmpty reports whether an optional stored credential is set.
func nonEmpty(s *string) bool { return s != nil && *s != "" }
