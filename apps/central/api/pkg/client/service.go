package client

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
	"net"
	"net/http"
	"strings"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/outbound"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service interface {
	Create(req *CreateClientRequest) (*Client, *ClientCredentials, error)
	GetByID(id uuid.UUID) (*Client, error)
	GetByClientID(clientID string) (*Client, error)
	// AllowsOrigin reports whether origin is registered (allowed_origins) on
	// an active client — that is, whether a browser application served from
	// it belongs to this deployment.
	AllowsOrigin(origin string) (bool, error)
	GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*Client, int64, error)
	// Update writes the patch to Authway's DB and propagates to Hydra.
	// SyncStatus reflects the upstream outcome — DB success with sync failure
	// is the common case the API caller previously couldn't observe.
	Update(id uuid.UUID, req *UpdateClientRequest) (*Client, SyncStatus, error)
	Delete(id uuid.UUID) (SyncStatus, error)
	List(limit, offset int) ([]*Client, int64, error)
	ValidateClient(clientID, clientSecret string) (*Client, error)
	RegenerateSecret(id uuid.UUID) (*ClientCredentials, SyncStatus, error)
	SyncAllClientsToHydra() (int, int, error) // returns (synced, failed, error)
	// EnsureAccountClient returns the tenant's account screen client,
	// registering it the first time (see system.go).
	EnsureAccountClient(tenantID uuid.UUID, loginUI string) (*Client, error)
	// DeleteSystemClients removes the clients Authway registered for a
	// tenant, when the tenant is deleted.
	DeleteSystemClients(tenantID uuid.UUID) error
	// InForce reports whether a client may be used: it is on, and so is its
	// tenant.
	InForce(c *Client) bool
	// TenantTurned registers the tenant's clients with the authorization
	// server anew after the tenant was turned off or on; turned off, it also
	// ends their tokens.
	TenantTurned(tenantID uuid.UUID, on bool) error
}

type service struct {
	db          *gorm.DB
	logger      *zap.Logger
	hydraClient *hydra.Client
	// outbound says which addresses the authorization server may be told to
	// call (a client's back-channel logout address); names resolves them.
	outbound outbound.Policy
	names    outbound.Resolver
}

// Option adjusts a client service.
type Option func(*service)

// CallableAddresses sets which addresses a client may have the authorization
// server call. Without it, only public https addresses.
func CallableAddresses(policy outbound.Policy) Option {
	return func(s *service) { s.outbound = policy }
}

func NewService(db *gorm.DB, logger *zap.Logger, hydraClient *hydra.Client, opts ...Option) Service {
	s := newService(db, logger, hydraClient)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func newService(db *gorm.DB, logger *zap.Logger, hydraClient *hydra.Client) *service {
	return &service{
		names:       net.DefaultResolver,
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}
}

func (s *service) Create(req *CreateClientRequest) (*Client, *ClientCredentials, error) {
	// Reject internally inconsistent client configurations up-front
	// (e.g. the ASP.NET-on-PKCE foot-gun — see validation.go).
	if cerr := validateClientConfig(req.Public, req.ClientSecret, req.GrantTypes, req.RedirectURIs, req.AllowedOrigins); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := validateAccessTokenStrategy(req.AccessTokenStrategy); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := validateAudience(req.Audience); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := validateApplicationURI("initiate_login_uri", req.InitiateLoginURI); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := validateApplicationURI("client_uri", req.ClientURI); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := validateApplicationURI("logo", req.Logo); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := validateSignInMethods(req.EnabledAuthProviders); cerr != nil {
		return nil, nil, cerr
	}
	if cerr := s.validateBackChannelLogoutURI(req.BackChannelLogoutURI); cerr != nil {
		return nil, nil, cerr
	}
	registeredKeys, cerr := s.validateAuthentication(req.Public, req.TokenEndpointAuthMethod, req.JWKS, req.JWKSURI)
	if cerr != nil {
		return nil, nil, cerr
	}
	keyAuth := req.TokenEndpointAuthMethod == AuthMethodPrivateKeyJWT
	if keyAuth && req.ClientSecret != "" {
		return nil, nil, &apierror.Refusal{
			Code:    "invalid_token_endpoint_auth_method",
			Field:   "client_secret",
			Message: "a client that authenticates with a key has no secret",
			Hint:    "Leave client_secret out. The client proves who it is by signing an assertion with its key.",
		}
	}

	// Validate tenant_id
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid tenant_id: %w", err)
	}

	// Verify tenant exists
	var tenantExists bool
	if err := s.db.Raw("SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND active = true)", tenantID).Scan(&tenantExists).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to verify tenant: %w", err)
	}
	if !tenantExists {
		return nil, nil, apierror.NewPublic("tenant not found or inactive")
	}

	// Use provided credentials or generate new ones
	// OAuth 2.0 RFC 6749 Section 2.1: Public clients cannot maintain credential confidentiality
	var clientID, clientSecret string

	if req.Public {
		// Public Client (SPA, Mobile) - client_secret not required
		if req.ClientID != "" {
			// Use provided client_id
			clientID = req.ClientID
			s.logger.Info("Using provided client_id for public client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
		} else {
			// Generate random client_id
			clientID = s.generateClientID()
			s.logger.Info("Generated client_id for public client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
		}
		clientSecret = "" // Public clients don't need secrets (use PKCE instead)

	} else if keyAuth {
		// A client that authenticates with a key: an id, and no secret.
		clientID = req.ClientID
		if clientID == "" {
			clientID = s.generateClientID()
		}
		clientSecret = ""
	} else {
		// Confidential Client (Backend) - both required or both generated
		if req.ClientID != "" && req.ClientSecret != "" {
			// Use provided credentials
			clientID = req.ClientID
			clientSecret = req.ClientSecret
			s.logger.Info("Using provided credentials for confidential client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))

		} else if req.ClientID != "" || req.ClientSecret != "" {
			// Partial credentials are a caller mistake, not a server fault —
			// surface as a structured apierror.Refusal so the handler maps it to 400.
			missing := "client_secret"
			if req.ClientID == "" {
				missing = "client_id"
			}
			return nil, nil, &apierror.Refusal{
				Code:    "confidential_client_partial_credentials",
				Field:   missing,
				Message: "Confidential clients must provide both client_id and client_secret, or neither",
				Hint:    fmt.Sprintf("Supply %s as well, or omit both and let Authway generate the pair.", missing),
			}

		} else {
			// Generate both credentials
			clientID = s.generateClientID()
			clientSecret = s.generateClientSecret()
			s.logger.Info("Generated credentials for confidential client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
		}
	}

	// A client_id the caller chose may be in use. One that is registered is
	// refused: answering with it would hand back credentials it does not have
	// and ignore what the request says. One that was deleted is gone from the
	// authorization server, and its row stays only because deletion is soft —
	// it is removed so the new client is registered from the request, whole.
	if req.ClientID != "" {
		var existing Client
		err := s.db.Unscoped().Where("client_id = ?", clientID).First(&existing).Error
		switch {
		case err == nil && !existing.DeletedAt.Valid:
			return nil, nil, &apierror.Refusal{
				Status:  http.StatusConflict,
				Code:    "client_id_taken",
				Field:   "client_id",
				Message: "a client with this client_id is already registered",
				Hint:    "Choose another client_id, or omit it and let Authway generate one.",
			}
		case err == nil:
			if err := s.db.Unscoped().Delete(&existing).Error; err != nil {
				return nil, nil, fmt.Errorf("failed to remove the deleted client: %w", err)
			}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return nil, nil, fmt.Errorf("failed to check existing client: %w", err)
		}
	}

	// The secret is returned to the caller once and registered with the
	// authorization server; only a hash of it stays here.
	secretHash := ""
	if clientSecret != "" {
		var err error
		if secretHash, err = hashSecret(clientSecret); err != nil {
			return nil, nil, err
		}
	}

	client := &Client{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ClientID:         clientID,
		ClientSecretHash: secretHash,
		Name:             req.Name,
		Description:      req.Description,
		Logo:             req.Logo,
		// nonNilURIs, not req.RedirectURIs: clients.redirect_uris is NOT NULL, and
		// GORM writes an explicit NULL for a nil slice rather than omitting the
		// column, so the DB default never applies. Now that M2M clients legitimately
		// have no redirect URIs, an empty array is the correct stored value.
		RedirectURIs: nonNilURIs(req.RedirectURIs),
		GrantTypes:   req.GrantTypes,
		Scopes:       req.Scopes,
		Public:       req.Public,
		Active:       true,
		System:       req.System,
	}

	// Set Google OAuth if provided
	if req.GoogleOAuthEnabled {
		client.GoogleOAuthEnabled = true
		if req.GoogleClientID != "" {
			client.GoogleClientID = &req.GoogleClientID
		}
		if req.GoogleClientSecret != "" {
			client.GoogleClientSecret = &req.GoogleClientSecret
		}
		if req.GoogleRedirectURI != "" {
			client.GoogleRedirectURI = &req.GoogleRedirectURI
		}
	}

	// Set GitHub OAuth if provided
	if req.GithubOAuthEnabled {
		client.GithubOAuthEnabled = true
		if req.GithubClientID != "" {
			client.GithubClientID = &req.GithubClientID
		}
		if req.GithubClientSecret != "" {
			client.GithubClientSecret = &req.GithubClientSecret
		}
	}

	// Set Microsoft OAuth if provided
	if req.MicrosoftOAuthEnabled {
		client.MicrosoftOAuthEnabled = true
		if req.MicrosoftClientID != "" {
			client.MicrosoftClientID = &req.MicrosoftClientID
		}
		if req.MicrosoftClientSecret != "" {
			client.MicrosoftClientSecret = &req.MicrosoftClientSecret
		}
		if req.MicrosoftTenantID != "" {
			client.MicrosoftTenantID = &req.MicrosoftTenantID
		}
	}

	// Set Apple OAuth if provided
	if req.AppleOAuthEnabled {
		client.AppleOAuthEnabled = true
		if req.AppleClientID != "" {
			client.AppleClientID = &req.AppleClientID
		}
		if req.AppleTeamID != "" {
			client.AppleTeamID = &req.AppleTeamID
		}
		if req.AppleKeyID != "" {
			client.AppleKeyID = &req.AppleKeyID
		}
		if req.ApplePrivateKey != "" {
			client.ApplePrivateKey = &req.ApplePrivateKey
		}
	}

	// Set Authentication Provider Settings
	// EnabledAuthProviders: default to email and google if not provided
	if len(req.EnabledAuthProviders) > 0 {
		client.EnabledAuthProviders = req.EnabledAuthProviders
	} else {
		client.EnabledAuthProviders = []string{"email", "google"}
	}
	// AllowEmailLogin: default to true
	if req.AllowEmailLogin != nil {
		client.AllowEmailLogin = *req.AllowEmailLogin
	} else {
		client.AllowEmailLogin = true
	}

	// Set allowed origins for CORS validation
	if len(req.AllowedOrigins) > 0 {
		client.AllowedOrigins = req.AllowedOrigins
	}

	client.Audience = req.Audience
	client.BackChannelLogoutURI = req.BackChannelLogoutURI
	client.InitiateLoginURI = req.InitiateLoginURI
	client.ClientURI = req.ClientURI
	if keyAuth {
		client.TokenEndpointAuthMethod = AuthMethodPrivateKeyJWT
		client.JWKS = registeredKeys
		client.JWKSURI = req.JWKSURI
	}

	// Logout Redirect Policy Configuration (Smart Defaults)
	// Philosophy: Minimize boilerplate - auto-populate from redirect_uris if not explicitly set
	if len(req.PostLogoutRedirectURIs) > 0 {
		// Use explicitly provided URIs
		client.PostLogoutRedirectURIs = req.PostLogoutRedirectURIs
	} else if len(req.RedirectURIs) > 0 {
		// Smart default: use redirect_uris as post_logout_redirect_uris
		// This follows the common pattern used by Auth0, Okta, and other providers
		client.PostLogoutRedirectURIs = req.RedirectURIs
		s.logger.Debug("Auto-populated post_logout_redirect_uris from redirect_uris",
			zap.String("client_id", clientID),
			zap.Strings("uris", req.RedirectURIs))
	}

	// Consent flow configuration (default: false — consent/logout screens shown)
	client.SkipConsent = req.SkipConsent
	client.SkipLogoutConsent = req.SkipLogoutConsent

	// Access token format (empty = inherit the deployment-wide strategy)
	if req.AccessTokenStrategy != "" {
		strategy := req.AccessTokenStrategy
		client.AccessTokenStrategy = &strategy
	}

	if err := s.db.Create(client).Error; err != nil {
		s.logger.Error("Failed to create client", zap.Error(err), zap.String("name", req.Name), zap.String("tenant_id", tenantID.String()))
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Register client in Hydra
	hydraClient := s.registration(client, clientSecret)

	_, err = s.hydraClient.CreateOAuth2Client(hydraClient)
	if err != nil {
		// Check if error is due to client already existing in Hydra
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "Conflict") {
			s.logger.Info("Client already exists in Hydra, skipping registration",
				zap.String("client_id", clientID))
			// Don't fail, client exists in both systems now
		} else {
			// Real Hydra error - rollback database creation
			s.db.Delete(client)
			s.logger.Error("Failed to register client in Hydra, rolled back database",
				zap.Error(err),
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
			// What the registration says was refused: the caller's to fix.
			var refused *hydra.ClientRefusedError
			if errors.As(err, &refused) {
				return nil, nil, refusedByAuthorizationServer(refused)
			}
			return nil, nil, fmt.Errorf("failed to register client in Hydra: %w", err)
		}
	}

	credentials := &ClientCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	s.logger.Info("Client created successfully in database and Hydra",
		zap.String("id", client.ID.String()),
		zap.String("client_id", clientID),
		zap.String("name", client.Name),
		zap.String("tenant_id", tenantID.String()))

	return client, credentials, nil
}

func (s *service) GetByID(id uuid.UUID) (*Client, error) {
	var client Client
	if err := s.db.Where("id = ?", id).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apierror.NewPublic("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return &client, nil
}

func (s *service) GetByClientID(clientID string) (*Client, error) {
	var client Client
	if err := s.db.Where("client_id = ?", clientID).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apierror.NewPublic("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return &client, nil
}

func (s *service) AllowsOrigin(origin string) (bool, error) {
	if origin == "" {
		return false, nil
	}
	var n int64
	err := s.db.Model(&Client{}).
		Where("active = ? AND ? = ANY(allowed_origins)", true, origin).
		Limit(1).Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("failed to look up the origin: %w", err)
	}
	return n > 0, nil
}

func (s *service) Update(id uuid.UUID, req *UpdateClientRequest) (*Client, SyncStatus, error) {
	var client Client
	if err := s.db.Where("id = ?", id).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, SyncStatus{}, apierror.NewPublic("client not found")
		}
		return nil, SyncStatus{}, fmt.Errorf("failed to get client: %w", err)
	}
	if client.System {
		return nil, SyncStatus{}, ErrSystemClient
	}
	wasActive := client.Active

	// Validate the *resulting* configuration after the patch is applied.
	// We compute the merged view of public/grant_types/origins/redirects
	// against the existing record so partial updates can't slip through.
	mergedPublic := client.Public
	if req.Public != nil {
		mergedPublic = *req.Public
	}
	mergedGrants := []string(client.GrantTypes)
	if req.GrantTypes != nil {
		mergedGrants = req.GrantTypes
	}
	mergedRedirects := []string(client.RedirectURIs)
	if req.RedirectURIs != nil {
		mergedRedirects = req.RedirectURIs
	}
	mergedOrigins := []string(client.AllowedOrigins)
	if req.AllowedOrigins != nil {
		mergedOrigins = req.AllowedOrigins
	}
	// Update doesn't expose client_secret; pass "" so the secret-on-public check
	// doesn't reject toggling a confidential client to public (the secret stays
	// in the DB but Hydra will be configured with token_endpoint_auth_method=none).
	if cerr := validateClientConfig(mergedPublic, "", mergedGrants, mergedRedirects, mergedOrigins); cerr != nil {
		return nil, SyncStatus{}, cerr
	}
	if req.AccessTokenStrategy != nil {
		if cerr := validateAccessTokenStrategy(*req.AccessTokenStrategy); cerr != nil {
			return nil, SyncStatus{}, cerr
		}
	}
	if cerr := validateAudience(req.Audience); cerr != nil {
		return nil, SyncStatus{}, cerr
	}
	// How a client authenticates is decided when it is registered. A change
	// would leave the authorization server holding whatever the other method
	// used — a secret that was meant to be gone would work again.
	if req.TokenEndpointAuthMethod != nil && *req.TokenEndpointAuthMethod != client.AuthMethod() {
		return nil, SyncStatus{}, &apierror.Refusal{
			Code:    "token_endpoint_auth_method_is_fixed",
			Field:   "token_endpoint_auth_method",
			Message: "how a client authenticates cannot be changed",
			Hint:    "Register a new client with the method wanted and move the application to it.",
		}
	}
	replacesKeys := len(req.JWKS) > 0 || req.JWKSURI != nil
	var replacedKeys, replacedKeysURI string
	if replacesKeys {
		if mergedPublic != client.Public || !client.AuthenticatesWithKey() {
			return nil, SyncStatus{}, &apierror.Refusal{
				Code:    "invalid_jwks",
				Field:   "jwks",
				Message: "a key is registered only for a client that authenticates with one",
				Hint:    jwksHint,
			}
		}
		if req.JWKSURI != nil {
			replacedKeysURI = *req.JWKSURI
		}
		var cerr *apierror.Refusal
		if replacedKeys, cerr = s.validateAuthentication(false, AuthMethodPrivateKeyJWT, req.JWKS, replacedKeysURI); cerr != nil {
			return nil, SyncStatus{}, cerr
		}
	} else if client.AuthenticatesWithKey() && mergedPublic {
		return nil, SyncStatus{}, &apierror.Refusal{
			Code:    "token_endpoint_auth_method_is_fixed",
			Field:   "public",
			Message: "a client that authenticates with a key cannot become a public client",
			Hint:    "Register a new public client and move the application to it.",
		}
	}
	for _, f := range []struct {
		field string
		value *string
	}{{"initiate_login_uri", req.InitiateLoginURI}, {"client_uri", req.ClientURI}, {"logo", req.Logo}} {
		if f.value != nil {
			if cerr := validateApplicationURI(f.field, *f.value); cerr != nil {
				return nil, SyncStatus{}, cerr
			}
		}
	}
	if cerr := validateSignInMethods(req.EnabledAuthProviders); cerr != nil {
		return nil, SyncStatus{}, cerr
	}
	if req.BackChannelLogoutURI != nil {
		if cerr := s.validateBackChannelLogoutURI(*req.BackChannelLogoutURI); cerr != nil {
			return nil, SyncStatus{}, cerr
		}
	}

	// Update fields
	// Note: For string fields, we update even if empty to allow clearing values
	// For array fields, nil means "not provided" while empty slice means "clear the values"
	if req.Name != "" {
		client.Name = req.Name
	}
	if req.Description != nil {
		client.Description = *req.Description
	}
	if req.Logo != nil {
		client.Logo = *req.Logo
	}

	// Array fields: update if slice is not nil (allows clearing with empty array)
	if req.RedirectURIs != nil {
		client.RedirectURIs = req.RedirectURIs
	}
	if req.GrantTypes != nil {
		client.GrantTypes = req.GrantTypes
	}
	if req.Scopes != nil {
		client.Scopes = req.Scopes
	}
	if req.Public != nil {
		client.Public = *req.Public
	}
	if req.Active != nil {
		client.Active = *req.Active
	}

	// Google OAuth settings
	if req.GoogleOAuthEnabled != nil {
		client.GoogleOAuthEnabled = *req.GoogleOAuthEnabled
	}
	if req.GoogleClientID != nil {
		client.GoogleClientID = req.GoogleClientID
	}
	if req.GoogleClientSecret != nil {
		client.GoogleClientSecret = req.GoogleClientSecret
	}
	if req.GoogleRedirectURI != nil {
		client.GoogleRedirectURI = req.GoogleRedirectURI
	}
	if req.GithubOAuthEnabled != nil {
		client.GithubOAuthEnabled = *req.GithubOAuthEnabled
	}
	if req.GithubClientID != nil {
		client.GithubClientID = req.GithubClientID
	}
	if req.GithubClientSecret != nil {
		client.GithubClientSecret = req.GithubClientSecret
	}

	// Update allowed origins if provided (nil = not provided, empty = clear)
	if req.AllowedOrigins != nil {
		client.AllowedOrigins = req.AllowedOrigins
	}

	// Logout redirect policy settings
	// PostLogoutRedirectURIs: nil = not provided, empty = clear values
	if req.PostLogoutRedirectURIs != nil {
		client.PostLogoutRedirectURIs = req.PostLogoutRedirectURIs
	}

	// Audience: nil = not provided, empty = clear
	if req.Audience != nil {
		client.Audience = req.Audience
	}
	if replacesKeys {
		client.JWKS = replacedKeys
		client.JWKSURI = replacedKeysURI
	}
	if req.InitiateLoginURI != nil {
		client.InitiateLoginURI = *req.InitiateLoginURI
	}
	if req.ClientURI != nil {
		client.ClientURI = *req.ClientURI
	}
	if req.BackChannelLogoutURI != nil {
		client.BackChannelLogoutURI = *req.BackChannelLogoutURI
	}

	// Consent flow configuration
	if req.SkipConsent != nil {
		client.SkipConsent = *req.SkipConsent
	}
	if req.SkipLogoutConsent != nil {
		client.SkipLogoutConsent = *req.SkipLogoutConsent
	}

	// Access token format: nil = not provided, "" = clear (inherit global).
	if req.AccessTokenStrategy != nil {
		if *req.AccessTokenStrategy == "" {
			client.AccessTokenStrategy = nil
		} else {
			client.AccessTokenStrategy = req.AccessTokenStrategy
		}
	}

	// Authentication Provider Settings
	// EnabledAuthProviders: nil = not provided (an empty list is refused above)
	if req.EnabledAuthProviders != nil {
		client.EnabledAuthProviders = req.EnabledAuthProviders
	}
	if req.AllowEmailLogin != nil {
		client.AllowEmailLogin = *req.AllowEmailLogin
	}

	// Microsoft OAuth settings
	if req.MicrosoftOAuthEnabled != nil {
		client.MicrosoftOAuthEnabled = *req.MicrosoftOAuthEnabled
	}
	if req.MicrosoftClientID != nil {
		client.MicrosoftClientID = req.MicrosoftClientID
	}
	if req.MicrosoftClientSecret != nil {
		client.MicrosoftClientSecret = req.MicrosoftClientSecret
	}
	if req.MicrosoftTenantID != nil {
		client.MicrosoftTenantID = req.MicrosoftTenantID
	}

	// Apple OAuth settings
	if req.AppleOAuthEnabled != nil {
		client.AppleOAuthEnabled = *req.AppleOAuthEnabled
	}
	if req.AppleClientID != nil {
		client.AppleClientID = req.AppleClientID
	}
	if req.AppleTeamID != nil {
		client.AppleTeamID = req.AppleTeamID
	}
	if req.AppleKeyID != nil {
		client.AppleKeyID = req.AppleKeyID
	}
	if req.ApplePrivateKey != nil {
		client.ApplePrivateKey = req.ApplePrivateKey
	}

	// The change is stored and passed on to the authorization server as one
	// step. If the server refuses what the change says — an address it will
	// not call, say — the caller has something to correct, and nothing is
	// kept: stored but not in force would be a client that says one thing and
	// does another. A server that cannot be reached is a different matter:
	// the change stands and the answer reports that it has yet to be passed on.
	syncStatus := SyncStatus{State: SyncStateOK}
	var refusal *apierror.Refusal
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&client).Error; err != nil {
			return err
		}
		// No secret is sent: the authorization server keeps the one it has
		// when an update names none (measured against v26.2.0).
		_, errHydra := s.hydraClient.UpdateOAuth2Client(client.ClientID, s.registration(&client, ""))
		var refused *hydra.ClientRefusedError
		if errors.As(errHydra, &refused) {
			refusal = refusedByAuthorizationServer(refused)
			return refusal
		}
		if errHydra != nil {
			syncStatus = SyncStatus{State: SyncStateFailed, Error: errHydra.Error()}
			s.logger.Warn("Failed to update client in Hydra (database updated)",
				zap.Error(errHydra),
				zap.String("client_id", client.ClientID))
		}
		return nil
	})
	if refusal != nil {
		return nil, SyncStatus{}, refusal
	}
	if err != nil {
		s.logger.Error("Failed to update client", zap.Error(err), zap.String("id", id.String()))
		return nil, SyncStatus{}, fmt.Errorf("failed to update client: %w", err)
	}

	// A client turned off keeps no token in force, so turning it on again does
	// not bring old sessions back. Its refresh tokens are refused while it is
	// off (it is registered with no grants).
	if wasActive && !client.Active && syncStatus.State == SyncStateOK {
		if err := s.endTokens(&client); err != nil {
			syncStatus = SyncStatus{State: SyncStateFailed, Error: err.Error()}
			s.logger.Warn("Failed to end the tokens of a client turned off",
				zap.Error(err), zap.String("client_id", client.ClientID))
		}
	}

	s.logger.Info("Client updated in database",
		zap.String("id", client.ID.String()),
		zap.String("hydra_sync_state", syncStatus.State))
	return &client, syncStatus, nil
}

func (s *service) Delete(id uuid.UUID) (SyncStatus, error) {
	// Get client first to retrieve client_id for Hydra deletion
	client, err := s.GetByID(id)
	if err != nil {
		return SyncStatus{}, err
	}
	if client.System {
		return SyncStatus{}, ErrSystemClient
	}
	return s.delete(client)
}

// delete removes a client from the authorization server, then from the
// database.
func (s *service) delete(client *Client) (SyncStatus, error) {
	id := client.ID
	// Delete from Hydra first
	syncStatus := SyncStatus{State: SyncStateOK}
	if err := s.hydraClient.DeleteOAuth2Client(client.ClientID); err != nil {
		syncStatus = SyncStatus{State: SyncStateFailed, Error: err.Error()}
		s.logger.Warn("Failed to delete client from Hydra (proceeding with database deletion)",
			zap.Error(err),
			zap.String("client_id", client.ClientID))
	}

	// Delete from database
	result := s.db.Delete(&Client{}, id)
	if result.Error != nil {
		s.logger.Error("Failed to delete client", zap.Error(result.Error), zap.String("id", id.String()))
		return syncStatus, fmt.Errorf("failed to delete client: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return syncStatus, apierror.NewPublic("client not found")
	}

	s.logger.Info("Client deleted from database",
		zap.String("id", id.String()),
		zap.String("hydra_sync_state", syncStatus.State))
	return syncStatus, nil
}

func (s *service) List(limit, offset int) ([]*Client, int64, error) {
	var clients []*Client
	var total int64

	// Get total count
	if err := s.db.Model(&Client{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count clients: %w", err)
	}

	// Get clients with pagination
	if err := s.db.Order("created_at DESC, id").Limit(limit).Offset(offset).Find(&clients).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list clients: %w", err)
	}

	return clients, total, nil
}

func (s *service) ValidateClient(clientID, clientSecret string) (*Client, error) {
	client, err := s.GetByClientID(clientID)
	if err != nil {
		return nil, err
	}

	if !client.Active {
		return nil, apierror.NewPublic("client is not active")
	}

	// For public clients, don't validate secret
	if client.Public {
		return client, nil
	}

	// Validate client secret for confidential clients
	if !client.secretMatches(clientSecret) {
		return nil, apierror.NewPublic("invalid client credentials")
	}

	return client, nil
}

func (s *service) RegenerateSecret(id uuid.UUID) (*ClientCredentials, SyncStatus, error) {
	client, err := s.GetByID(id)
	if err != nil {
		return nil, SyncStatus{}, err
	}
	if client.System {
		return nil, SyncStatus{}, ErrSystemClient
	}
	if client.Public {
		return nil, SyncStatus{}, &apierror.Refusal{
			Code:    "public_client_has_no_secret",
			Field:   "public",
			Message: "Public clients have no secret to regenerate",
			Hint:    "A public client authenticates with PKCE instead of a secret. Register a confidential client if the application can keep a secret.",
		}
	}
	if client.AuthenticatesWithKey() {
		return nil, SyncStatus{}, &apierror.Refusal{
			Code:    "client_has_no_secret",
			Field:   "token_endpoint_auth_method",
			Message: "This client authenticates with a key and has no secret",
			Hint:    "To change its key, update jwks or jwks_uri.",
		}
	}

	// Generate new secret
	newSecret := s.generateClientSecret()

	newHash, err := hashSecret(newSecret)
	if err != nil {
		return nil, SyncStatus{}, err
	}
	if err := s.db.Model(client).Updates(map[string]any{"client_secret": "", "client_secret_hash": newHash}).Error; err != nil {
		s.logger.Error("Failed to regenerate client secret", zap.Error(err), zap.String("id", id.String()))
		return nil, SyncStatus{}, fmt.Errorf("failed to regenerate client secret: %w", err)
	}

	// Update secret in Hydra
	hydraUpdate := s.registration(client, newSecret)

	syncStatus := SyncStatus{State: SyncStateOK}
	if _, errHydra := s.hydraClient.UpdateOAuth2Client(client.ClientID, hydraUpdate); errHydra != nil {
		// Critical: secret rotation drift means Authway DB and Hydra disagree
		// on the client secret — every subsequent token request will fail until
		// drift is reconciled. Caller MUST observe sync_status=failed.
		syncStatus = SyncStatus{State: SyncStateFailed, Error: errHydra.Error()}
		s.logger.Warn("Failed to update client secret in Hydra (database updated — DRIFT)",
			zap.Error(errHydra),
			zap.String("client_id", client.ClientID))
	}

	credentials := &ClientCredentials{
		ClientID:     client.ClientID,
		ClientSecret: newSecret,
	}

	s.logger.Info("Client secret regenerated in database",
		zap.String("id", client.ID.String()),
		zap.String("hydra_sync_state", syncStatus.State))
	return credentials, syncStatus, nil
}

func (s *service) generateClientID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// crypto/rand failure means the OS entropy source is broken; minting a
		// predictable client_id from zeroed bytes is worse than crashing.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("authway_%s", base64.URLEncoding.EncodeToString(bytes)[:22])
}

func (s *service) generateClientSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Same reasoning as generateClientID: a zeroed-bytes secret is a
		// catastrophic credential leak. Crash instead.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return strings.ReplaceAll(base64.URLEncoding.EncodeToString(bytes), "=", "")
}

// GetByTenant retrieves all clients for a specific tenant with pagination
func (s *service) GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*Client, int64, error) {
	var clients []*Client
	var total int64

	// Get total count for this tenant
	if err := s.db.Model(&Client{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count clients: %w", err)
	}

	// Get clients with pagination
	if err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC, id").Limit(limit).Offset(offset).Find(&clients).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list clients: %w", err)
	}

	return clients, total, nil
}

// nonNilURIs guarantees a JSON `[]` rather than `null` on the wire to Hydra.
// Machine-to-machine clients legitimately have no redirect_uris (see
// validateRedirectURIRequirement), and a nil pq.StringArray would otherwise
// marshal to null — an ambiguity no client-registration payload should carry.
func nonNilURIs(uris []string) []string {
	if uris == nil {
		return []string{}
	}
	return uris
}

// hydraRegistration is the client as the authorization server stores it.
// Creating, updating and regenerating the secret all send this one shape: an
// update replaces the whole client there, so a field one of them left out
// would be reset.
func (c *Client) hydraRegistration(secret string) *hydra.OAuth2Client {
	authMethod := c.AuthMethod()

	// A client that names no post-logout address may return to its redirect
	// addresses, so that signing out works without further configuration.
	postLogoutURIs := []string(c.PostLogoutRedirectURIs)
	if len(postLogoutURIs) == 0 && len(c.RedirectURIs) > 0 {
		postLogoutURIs = c.RedirectURIs
	}

	return &hydra.OAuth2Client{
		ClientID:               c.ClientID,
		ClientSecret:           secret,
		ClientName:             c.Name,
		RedirectUris:           nonNilURIs(c.RedirectURIs),
		PostLogoutRedirectUris: postLogoutURIs,
		Audience:               nonNil(c.Audience),
		// The logout token names the session (sid), which is what an
		// application has from the ID token to match it against.
		BackChannelLogoutURI:             c.BackChannelLogoutURI,
		BackChannelLogoutSessionRequired: c.BackChannelLogoutURI != "",
		ClientURI:                        c.ClientURI,
		LogoURI:                          c.Logo,
		GrantTypes:                       c.grantsInForce(),
		ResponseTypes:                    []string{"code"},
		Scope:                            strings.Join(c.Scopes, " "),
		TokenEndpointAuthMethod:          authMethod,
		JWKS:                             c.registeredKeys(),
		JWKSURI:                          c.JWKSURI,
		SkipConsent:                      c.SkipConsent,
		SkipLogoutConsent:                c.SkipLogoutConsent,
		AccessTokenStrategy:              derefStrategy(c.AccessTokenStrategy),
	}
}

// tenantOn reports whether a tenant is on. A tenant that cannot be read is
// taken as on: what decides is the client's own state, and the error shows
// where the tenant is needed next.
func (s *service) tenantOn(tenantID uuid.UUID) bool {
	var active []bool
	if err := s.db.Table("tenants").Where("id = ?", tenantID).Pluck("active", &active).Error; err != nil || len(active) == 0 {
		return true
	}
	return active[0]
}

func (s *service) InForce(c *Client) bool {
	return c.Active && s.tenantOn(c.TenantID)
}

// registration is what the authorization server is to hold for c: the
// client's own registration, with no grants while its tenant is off.
func (s *service) registration(c *Client, secret string) *hydra.OAuth2Client {
	reg := c.hydraRegistration(secret)
	if !s.tenantOn(c.TenantID) {
		reg.GrantTypes = []string{}
	}
	return reg
}

func (s *service) TenantTurned(tenantID uuid.UUID, on bool) error {
	var clients []*Client
	if err := s.db.Where("tenant_id = ?", tenantID).Find(&clients).Error; err != nil {
		return fmt.Errorf("failed to list the tenant's clients: %w", err)
	}
	var firstErr error
	for _, c := range clients {
		if _, err := s.hydraClient.UpdateOAuth2Client(c.ClientID, s.registration(c, "")); err != nil {
			s.logger.Warn("Failed to register a client anew after its tenant was turned", zap.Error(err), zap.String("client_id", c.ClientID))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !on && c.Active {
			if err := s.endTokens(c); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// endTokens ends the tokens issued to a client: its access tokens, and what
// each user of its tenant granted it — which ends their refresh tokens.
func (s *service) endTokens(client *Client) error {
	if err := s.hydraClient.DeleteClientTokens(client.ClientID); err != nil {
		return err
	}
	var subjects []uuid.UUID
	if err := s.db.Table("users").Where("tenant_id = ?", client.TenantID).Pluck("id", &subjects).Error; err != nil {
		return fmt.Errorf("failed to list the tenant's users: %w", err)
	}
	// One call per user (the engine has no call for a client's consents), so
	// a few at a time: a tenant of thousands of users takes seconds, not
	// minutes.
	var g errgroup.Group
	g.SetLimit(revocationsAtOnce)
	for _, subject := range subjects {
		g.Go(func() error {
			if err := s.hydraClient.RevokeClientConsent(subject.String(), client.ClientID); err != nil {
				return fmt.Errorf("failed to revoke a consent: %w", err)
			}
			return nil
		})
	}
	return g.Wait()
}

// revocationsAtOnce bounds the calls endTokens makes to the authorization
// server at the same time.
const revocationsAtOnce = 16

// grantsInForce are the grants the authorization server is to serve the
// client: none while the client is turned off. The engine checks a client's
// grants on every token request — a refresh, client credentials, a device
// code — and an authorization request, so a client registered with none gets
// nothing from it; the stored grants come back when it is turned on.
func (c *Client) grantsInForce() []string {
	if !c.Active {
		return []string{}
	}
	return c.GrantTypes
}

// derefStrategy flattens the optional per-client access token strategy for the
// Hydra payload. nil (not pinned) becomes "", which the omitempty tag drops —
// and since client update is a PUT full-replace, that also un-pins a client that
// was previously pinned, restoring the deployment-wide strategy.
func derefStrategy(strategy *string) string {
	if strategy == nil {
		return ""
	}
	return *strategy
}

// SyncAllClientsToHydra synchronizes all clients' post_logout_redirect_uris to Hydra
// This is useful for one-time migration of existing clients that have empty post_logout_redirect_uris in Hydra
// For each client, if post_logout_redirect_uris is empty, it uses redirect_uris as fallback
func (s *service) SyncAllClientsToHydra() (int, int, error) {
	s.logger.Info("Starting Hydra sync for all clients")

	var clients []*Client
	if err := s.db.Find(&clients).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to fetch clients: %w", err)
	}

	synced := 0
	failed := 0

	for _, client := range clients {
		// Determine post_logout_redirect_uris to use
		hydraPostLogoutURIs := client.PostLogoutRedirectURIs
		if len(hydraPostLogoutURIs) == 0 && len(client.RedirectURIs) > 0 {
			hydraPostLogoutURIs = client.RedirectURIs
			s.logger.Debug("Using redirect_uris as fallback for empty post_logout_redirect_uris",
				zap.String("client_id", client.ClientID))
		}

		// Skip if still no URIs to set
		if len(hydraPostLogoutURIs) == 0 {
			s.logger.Debug("Skipping client with no redirect URIs",
				zap.String("client_id", client.ClientID))
			continue
		}

		// Update Hydra with the URIs
		err := s.hydraClient.UpdateClient(client.ClientID, map[string]any{
			"post_logout_redirect_uris": hydraPostLogoutURIs,
		})

		if err != nil {
			s.logger.Error("Failed to sync client to Hydra",
				zap.String("client_id", client.ClientID),
				zap.Error(err))
			failed++
			continue
		}

		s.logger.Debug("Successfully synced client to Hydra",
			zap.String("client_id", client.ClientID),
			zap.Strings("post_logout_redirect_uris", hydraPostLogoutURIs))
		synced++
	}

	s.logger.Info("Hydra sync completed",
		zap.Int("synced", synced),
		zap.Int("failed", failed),
		zap.Int("total", len(clients)))

	return synced, failed, nil
}
