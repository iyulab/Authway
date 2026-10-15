package client

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"authway/apps/central/api/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ErrSystemClient refuses a change to an application Authway itself owns.
var ErrSystemClient = apierror.NewPublic("this application is part of Authway and cannot be changed or deleted")

// AccountClientName is what the account screen's client is called.
const AccountClientName = "Account"

// accountClientScopes are what the account screen asks for: who is signed in.
var accountClientScopes = []string{"openid", "email", "profile"}

// accountClientProviders are every sign-in method, so a user signs in to the
// account screen however they sign in to the tenant's applications; the login
// screen offers only those the deployment has configured.
var accountClientProviders = []string{"email", "magic_link", "google", "github", "microsoft", "apple"}

// uniqueViolation is the SQLSTATE of a row refused by a unique index.
const uniqueViolation = "23505"

// EnsureAccountClient returns the tenant's account screen client, registering
// it the first time: a public client (PKCE) whose sign-in comes back to
// <loginUI>/account/callback, without a consent screen. Every tenant gets the
// same one by the same path, existing tenants included. Two first requests that
// race end with one client — the unique index refuses the second row before the
// authorization server hears of it, and the loser reads the winner's.
func (s *service) EnsureAccountClient(tenantID uuid.UUID, loginUI string) (*Client, error) {
	if existing, err := s.systemClient(tenantID); err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return existing, err
	}

	base := strings.TrimRight(loginUI, "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("the login UI address %q is not an absolute URL", loginUI)
	}
	created, _, err := s.Create(&CreateClientRequest{
		System:                 true,
		TenantID:               tenantID.String(),
		Name:                   AccountClientName,
		Description:            "Where users manage their own sign-in: the second factor and recovery codes.",
		Public:                 true,
		RedirectURIs:           []string{base + "/account/callback"},
		PostLogoutRedirectURIs: []string{base + "/account"},
		AllowedOrigins:         []string{u.Scheme + "://" + u.Host},
		GrantTypes:             []string{"authorization_code"},
		Scopes:                 accountClientScopes,
		EnabledAuthProviders:   accountClientProviders,
		SkipConsent:            true,
		SkipLogoutConsent:      true,
	})
	var refused *pgconn.PgError
	if errors.As(err, &refused) && refused.Code == uniqueViolation {
		return s.systemClient(tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to register the account client: %w", err)
	}
	return created, nil
}

// DeleteSystemClients removes the applications Authway registered for a tenant
// — from the authorization server too — when the tenant itself is deleted.
func (s *service) DeleteSystemClients(tenantID uuid.UUID) error {
	var system []Client
	if err := s.db.Where("tenant_id = ? AND is_system", tenantID).Find(&system).Error; err != nil {
		return fmt.Errorf("failed to read the tenant's system clients: %w", err)
	}
	for _, c := range system {
		if _, err := s.delete(&c); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) systemClient(tenantID uuid.UUID) (*Client, error) {
	var c Client
	if err := s.db.Where("tenant_id = ? AND is_system", tenantID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
