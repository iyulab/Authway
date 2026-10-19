package client

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"authway/apps/central/api/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ErrSystemClient refuses a change to an application Authway itself owns.
var ErrSystemClient = apierror.NewPublic("this application is part of Authway and cannot be changed or deleted")

// The kinds of application Authway registers for itself.
const (
	// SystemKindAccount is the account screen's client, in every tenant.
	SystemKindAccount = "account"
	// SystemKindConsole is the console's client, in the system tenant only.
	SystemKindConsole = "console"
)

// AccountClientName is what the account screen's client is called.
const AccountClientName = "Account"

// ConsoleClientName is what the console's client is called.
const ConsoleClientName = "Console"

// ConsoleAudience is the audience of the console's access tokens: the
// administration API takes a console account's token only with it.
const ConsoleAudience = "urn:authway:console"

// accountClientScopes are what the account screen asks for: who is signed in.
var accountClientScopes = []string{"openid", "email", "profile"}

// ConsoleClientScopes are what the console asks for: who is signed in.
var ConsoleClientScopes = []string{"openid", "email", "profile"}

// systemClientProviders are every sign-in method, so a user signs in to a
// system client however they sign in to the tenant's applications; the login
// screen offers only those the deployment has configured.
var systemClientProviders = []string{"email", "magic_link", "google", "github", "microsoft", "apple"}

// uniqueViolation is the SQLSTATE of a row refused by a unique index.
const uniqueViolation = "23505"

// EnsureAccountClient returns the tenant's account screen client, registering
// it the first time: a public client (PKCE) whose sign-in comes back to
// <loginUI>/account/callback, without a consent screen. Every tenant gets the
// same one by the same path, existing tenants included.
func (s *service) EnsureAccountClient(tenantID uuid.UUID, loginUI string) (*Client, error) {
	base, origin, err := absolute("login UI", loginUI)
	if err != nil {
		return nil, err
	}
	return s.ensureSystemClient(tenantID, &CreateClientRequest{
		SystemKind:             SystemKindAccount,
		Name:                   AccountClientName,
		Description:            "Where users manage their own sign-in: the second factor and recovery codes.",
		RedirectURIs:           []string{base + "/account/callback"},
		PostLogoutRedirectURIs: []string{base + "/account"},
		AllowedOrigins:         []string{origin},
		Scopes:                 accountClientScopes,
	})
}

// EnsureConsoleClient returns the client the console signs console accounts in
// with, registering it the first time: a public client (PKCE) of the system
// tenant whose sign-in comes back to <console>/callback, without a consent
// screen, issuing tokens for ConsoleAudience. It has no refresh token: the
// console's tabs would rotate one refresh token among them and end the session
// as a theft; the console renews its token by signing in again silently.
func (s *service) EnsureConsoleClient(systemTenantID uuid.UUID, consoleURL string) (*Client, error) {
	base, origin, err := absolute("console", consoleURL)
	if err != nil {
		return nil, err
	}
	return s.ensureSystemClient(systemTenantID, &CreateClientRequest{
		SystemKind:             SystemKindConsole,
		Name:                   ConsoleClientName,
		Description:            "Where console accounts administer Authway.",
		RedirectURIs:           []string{base + "/callback"},
		PostLogoutRedirectURIs: []string{base + "/"},
		AllowedOrigins:         []string{origin},
		Scopes:                 ConsoleClientScopes,
		Audience:               []string{ConsoleAudience},
	})
}

// ensureSystemClient returns the tenant's system client of want's kind,
// registering it the first time. One that exists is brought to want's
// addresses and audience — they follow the deployment's configuration, and a
// moved login UI or console would otherwise be left with a client that sends
// its users back to the old address. Two first requests that race end with one
// client: the unique index refuses the second row before the authorization
// server hears of it, and the loser reads the winner's.
func (s *service) ensureSystemClient(tenantID uuid.UUID, want *CreateClientRequest) (*Client, error) {
	existing, err := s.systemClient(tenantID, want.SystemKind)
	if err == nil {
		return s.followConfiguration(existing, want)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	want.System = true
	want.TenantID = tenantID.String()
	want.Public = true
	want.GrantTypes = []string{"authorization_code"}
	want.EnabledAuthProviders = systemClientProviders
	want.SkipConsent = true
	want.SkipLogoutConsent = true
	created, _, err := s.Create(want)
	var refused *pgconn.PgError
	if errors.As(err, &refused) && refused.Code == uniqueViolation {
		return s.systemClient(tenantID, want.SystemKind)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to register the %s client: %w", want.SystemKind, err)
	}
	return created, nil
}

// followConfiguration brings a system client's addresses and audience to
// want's, in the store and at the authorization server.
func (s *service) followConfiguration(c *Client, want *CreateClientRequest) (*Client, error) {
	if slices.Equal([]string(c.RedirectURIs), want.RedirectURIs) &&
		slices.Equal([]string(c.PostLogoutRedirectURIs), want.PostLogoutRedirectURIs) &&
		slices.Equal([]string(c.AllowedOrigins), want.AllowedOrigins) &&
		slices.Equal([]string(c.Audience), nonNilList(want.Audience)) {
		return c, nil
	}
	c.RedirectURIs = want.RedirectURIs
	c.PostLogoutRedirectURIs = want.PostLogoutRedirectURIs
	c.AllowedOrigins = want.AllowedOrigins
	c.Audience = nonNilList(want.Audience)
	if _, err := s.hydraClient.UpdateOAuth2Client(c.ClientID, s.registration(c, "")); err != nil {
		return nil, fmt.Errorf("failed to register the %s client's addresses anew: %w", c.SystemKind, err)
	}
	if err := s.db.Model(c).Select("redirect_uris", "post_logout_redirect_uris", "allowed_origins", "audience").Updates(c).Error; err != nil {
		return nil, fmt.Errorf("failed to store the %s client's addresses: %w", c.SystemKind, err)
	}
	return c, nil
}

// absolute returns address without a trailing slash and its origin, or an
// error naming what the address is of.
func absolute(of, address string) (base, origin string, err error) {
	base = strings.TrimRight(address, "/")
	u, perr := url.Parse(base)
	if perr != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("the %s address %q is not an absolute URL", of, address)
	}
	return base, u.Scheme + "://" + u.Host, nil
}

func nonNilList(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
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

func (s *service) systemClient(tenantID uuid.UUID, kind string) (*Client, error) {
	var c Client
	if err := s.db.Where("tenant_id = ? AND is_system AND system_kind = ?", tenantID, kind).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
