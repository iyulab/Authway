package client

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Client represents an OAuth 2.0 client application
// Each client belongs to one tenant
type Client struct {
	ID           uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID     uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	ClientID     string         `json:"client_id" gorm:"uniqueIndex;not null"`
	ClientSecret string         `json:"-" gorm:"not null"`
	Name         string         `json:"name" gorm:"not null"`
	Description  string         `json:"description"`
	Website      string         `json:"website"`
	Logo         string         `json:"logo"`
	RedirectURIs pq.StringArray `json:"redirect_uris" gorm:"type:text[]"`
	GrantTypes   pq.StringArray `json:"grant_types" gorm:"type:text[]"`
	Scopes       pq.StringArray `json:"scopes" gorm:"type:text[]"`
	Public       bool           `json:"public" gorm:"default:false"`
	Active       bool           `json:"active" gorm:"default:true"`

	// Client-specific Google OAuth (optional - if enabled, uses client settings; otherwise uses Authway common OAuth)
	GoogleOAuthEnabled bool    `json:"google_oauth_enabled" gorm:"column:google_oauth_enabled;default:false"`
	GoogleClientID     *string `json:"-" gorm:"column:google_client_id;null"`
	GoogleClientSecret *string `json:"-" gorm:"column:google_client_secret;null"`
	GoogleRedirectURI  *string `json:"google_redirect_uri" gorm:"column:google_redirect_uri;null"`

	// Client-specific GitHub OAuth (optional)
	GithubOAuthEnabled bool    `json:"github_oauth_enabled" gorm:"column:github_oauth_enabled;default:false"`
	GithubClientID     *string `json:"-" gorm:"column:github_client_id;null"`
	GithubClientSecret *string `json:"-" gorm:"column:github_client_secret;null"`

	// CORS Allowed Origins for dynamic CORS validation by reverse proxy
	// Used to validate browser requests to /oauth2/token endpoint
	AllowedOrigins pq.StringArray `json:"allowed_origins" gorm:"type:text[];column:allowed_origins;default:'{}'"`

	// Logout Redirect Policy Configuration
	// Controls validation strictness for post_logout_redirect_uri parameter
	PostLogoutRedirectURIs pq.StringArray `json:"post_logout_redirect_uris" gorm:"type:text[];column:post_logout_redirect_uris;default:'{}'"`
	LogoutRedirectPolicy   string         `json:"logout_redirect_policy" gorm:"column:logout_redirect_policy;default:'strict'"` // strict, lenient, disabled
	DefaultLogoutURI       *string        `json:"default_logout_uri" gorm:"column:default_logout_uri;null"`
	AllowWildcardLogout    bool           `json:"allow_wildcard_logout" gorm:"column:allow_wildcard_logout;default:false"`

	// Authentication Provider Settings
	// Controls which authentication methods are available for this client
	EnabledAuthProviders pq.StringArray `json:"enabled_auth_providers" gorm:"type:text[];column:enabled_auth_providers;default:'{email,google}'"`
	AllowEmailSignup     bool           `json:"allow_email_signup" gorm:"column:allow_email_signup;default:true"`
	AllowEmailLogin      bool           `json:"allow_email_login" gorm:"column:allow_email_login;default:true"`

	// Consent Flow Configuration
	// SkipConsent bypasses the OAuth consent screen; SkipLogoutConsent bypasses
	// the logout confirmation. Intended for first-party/trusted clients so users
	// aren't prompted on every login. Propagated to Hydra's per-client
	// skip_consent / skip_logout_consent (admin-API-only) fields on sync.
	SkipConsent       bool `json:"skip_consent" gorm:"column:skip_consent;default:false"`
	SkipLogoutConsent bool `json:"skip_logout_consent" gorm:"column:skip_logout_consent;default:false"`

	// Microsoft OAuth settings (optional - client-specific credentials)
	MicrosoftOAuthEnabled bool    `json:"microsoft_oauth_enabled" gorm:"column:microsoft_oauth_enabled;default:false"`
	MicrosoftClientID     *string `json:"-" gorm:"column:microsoft_client_id;null"`
	MicrosoftClientSecret *string `json:"-" gorm:"column:microsoft_client_secret;null"`
	MicrosoftTenantID     *string `json:"microsoft_tenant_id" gorm:"column:microsoft_tenant_id;null"`

	// Apple OAuth settings (optional - client-specific credentials)
	AppleOAuthEnabled bool    `json:"apple_oauth_enabled" gorm:"column:apple_oauth_enabled;default:false"`
	AppleClientID     *string `json:"-" gorm:"column:apple_client_id;null"`
	AppleTeamID       *string `json:"-" gorm:"column:apple_team_id;null"`
	AppleKeyID        *string `json:"-" gorm:"column:apple_key_id;null"`
	ApplePrivateKey   *string `json:"-" gorm:"column:apple_private_key;null"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// BeforeCreate sets UUID if not provided
func (c *Client) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// PublicClient returns client data safe for public consumption
type PublicClient struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	ClientID     string    `json:"client_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Website      string    `json:"website"`
	Logo         string    `json:"logo"`
	RedirectURIs []string  `json:"redirect_uris"`
	GrantTypes   []string  `json:"grant_types"`
	Scopes       []string  `json:"scopes"`
	Public       bool      `json:"public"`
	Active       bool      `json:"active"`

	// OAuth Settings (public fields only)
	GoogleOAuthEnabled bool    `json:"google_oauth_enabled"`
	GoogleRedirectURI  *string `json:"google_redirect_uri"`
	GithubOAuthEnabled bool    `json:"github_oauth_enabled"`

	// CORS Allowed Origins
	AllowedOrigins []string `json:"allowed_origins"`

	// Logout Redirect Policy
	PostLogoutRedirectURIs []string `json:"post_logout_redirect_uris"`
	LogoutRedirectPolicy   string   `json:"logout_redirect_policy"` // strict, lenient, disabled
	DefaultLogoutURI       *string  `json:"default_logout_uri"`
	AllowWildcardLogout    bool     `json:"allow_wildcard_logout"`

	// Authentication Provider Settings
	EnabledAuthProviders []string `json:"enabled_auth_providers"`
	AllowEmailSignup     bool     `json:"allow_email_signup"`
	AllowEmailLogin      bool     `json:"allow_email_login"`

	// Microsoft OAuth (public fields)
	MicrosoftOAuthEnabled bool    `json:"microsoft_oauth_enabled"`
	MicrosoftTenantID     *string `json:"microsoft_tenant_id"`

	// Apple OAuth (public fields)
	AppleOAuthEnabled bool `json:"apple_oauth_enabled"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToPublic converts Client to PublicClient
func (c *Client) ToPublic() PublicClient {
	return PublicClient{
		ID:           c.ID,
		TenantID:     c.TenantID,
		ClientID:     c.ClientID,
		Name:         c.Name,
		Description:  c.Description,
		Website:      c.Website,
		Logo:         c.Logo,
		RedirectURIs: c.RedirectURIs,
		GrantTypes:   c.GrantTypes,
		Scopes:       c.Scopes,
		Public:       c.Public,
		Active:       c.Active,

		// OAuth public fields
		GoogleOAuthEnabled: c.GoogleOAuthEnabled,
		GoogleRedirectURI:  c.GoogleRedirectURI,
		GithubOAuthEnabled: c.GithubOAuthEnabled,

		// CORS origins
		AllowedOrigins: c.AllowedOrigins,

		// Logout policy
		PostLogoutRedirectURIs: c.PostLogoutRedirectURIs,
		LogoutRedirectPolicy:   c.LogoutRedirectPolicy,
		DefaultLogoutURI:       c.DefaultLogoutURI,
		AllowWildcardLogout:    c.AllowWildcardLogout,

		// Auth provider settings
		EnabledAuthProviders: c.EnabledAuthProviders,
		AllowEmailSignup:     c.AllowEmailSignup,
		AllowEmailLogin:      c.AllowEmailLogin,

		// Microsoft OAuth public fields
		MicrosoftOAuthEnabled: c.MicrosoftOAuthEnabled,
		MicrosoftTenantID:     c.MicrosoftTenantID,

		// Apple OAuth public fields
		AppleOAuthEnabled: c.AppleOAuthEnabled,

		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// CreateClientRequest represents the request to create a new OAuth client
type CreateClientRequest struct {
	TenantID string `json:"tenant_id" validate:"required,uuid"` // Required
	Name     string `json:"name" validate:"required"`
	Public   bool   `json:"public"` // true: Public client (SPA, Mobile), false: Confidential client (Backend)

	// Client Credentials:
	// - Public clients: Only client_id is used (client_secret ignored/not required)
	// - Confidential clients: Both client_id and client_secret must be provided together, or both omitted for auto-generation
	ClientID     string `json:"client_id"`     // Optional: Custom client_id (for both public and confidential)
	ClientSecret string `json:"client_secret"` // Optional: Required only for confidential clients (ignored for public clients)

	Description  string   `json:"description"`
	Website      string   `json:"website" validate:"omitempty,url"`
	Logo         string   `json:"logo" validate:"omitempty,url"`
	RedirectURIs []string `json:"redirect_uris" validate:"required,min=1,dive,url"`
	GrantTypes   []string `json:"grant_types" validate:"required,min=1"`
	Scopes       []string `json:"scopes" validate:"required,min=1"`

	// Google OAuth Settings (optional)
	GoogleOAuthEnabled bool   `json:"google_oauth_enabled"`
	GoogleClientID     string `json:"google_client_id" validate:"required_with=GoogleOAuthEnabled"`
	GoogleClientSecret string `json:"google_client_secret" validate:"required_with=GoogleOAuthEnabled"`
	GoogleRedirectURI  string `json:"google_redirect_uri" validate:"required_with=GoogleOAuthEnabled,omitempty,url"`

	// GitHub OAuth Settings (optional)
	GithubOAuthEnabled bool   `json:"github_oauth_enabled"`
	GithubClientID     string `json:"github_client_id" validate:"required_with=GithubOAuthEnabled"`
	GithubClientSecret string `json:"github_client_secret" validate:"required_with=GithubOAuthEnabled"`

	// CORS Allowed Origins for browser-based OAuth flows
	// Required for SPA clients using Authorization Code Flow with PKCE
	// Example: ["https://app.example.com", "https://staging.example.com"]
	AllowedOrigins []string `json:"allowed_origins" validate:"omitempty,dive,url"`

	// Logout Redirect Policy Configuration
	PostLogoutRedirectURIs []string `json:"post_logout_redirect_uris" validate:"omitempty,dive,url"`
	LogoutRedirectPolicy   string   `json:"logout_redirect_policy" validate:"omitempty,oneof=strict lenient disabled"`
	DefaultLogoutURI       string   `json:"default_logout_uri" validate:"omitempty,url"`
	AllowWildcardLogout    bool     `json:"allow_wildcard_logout"`

	// Authentication Provider Settings
	// EnabledAuthProviders: array of provider names (email, google, github, microsoft, apple)
	EnabledAuthProviders []string `json:"enabled_auth_providers"`
	AllowEmailSignup     *bool    `json:"allow_email_signup"` // Pointer to use default if not provided
	AllowEmailLogin      *bool    `json:"allow_email_login"`  // Pointer to use default if not provided

	// Consent Flow Configuration (default: false — consent/logout screens shown)
	SkipConsent       bool `json:"skip_consent"`
	SkipLogoutConsent bool `json:"skip_logout_consent"`

	// Microsoft OAuth Settings (optional - client-specific credentials)
	MicrosoftOAuthEnabled bool   `json:"microsoft_oauth_enabled"`
	MicrosoftClientID     string `json:"microsoft_client_id"`
	MicrosoftClientSecret string `json:"microsoft_client_secret"`
	MicrosoftTenantID     string `json:"microsoft_tenant_id"`

	// Apple OAuth Settings (optional - client-specific credentials)
	AppleOAuthEnabled bool   `json:"apple_oauth_enabled"`
	AppleClientID     string `json:"apple_client_id"`
	AppleTeamID       string `json:"apple_team_id"`
	AppleKeyID        string `json:"apple_key_id"`
	ApplePrivateKey   string `json:"apple_private_key"`
}

// UpdateClientRequest represents the request to update an OAuth client
type UpdateClientRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Website      string   `json:"website" validate:"omitempty,url"`
	Logo         string   `json:"logo" validate:"omitempty,url"`
	RedirectURIs []string `json:"redirect_uris" validate:"omitempty,min=1,dive,url"`
	GrantTypes   []string `json:"grant_types" validate:"omitempty,min=1"`
	Scopes       []string `json:"scopes" validate:"omitempty,min=1"`
	Public       *bool    `json:"public"`  // Pointer to allow explicit false
	Active       *bool    `json:"active"`  // Pointer to allow explicit false

	// Google OAuth Settings (optional)
	GoogleOAuthEnabled *bool   `json:"google_oauth_enabled"` // Pointer to allow explicit false
	GoogleClientID     *string `json:"google_client_id"`
	GoogleClientSecret *string `json:"google_client_secret"`
	GoogleRedirectURI  *string `json:"google_redirect_uri" validate:"omitempty,url"`

	// CORS Allowed Origins
	AllowedOrigins []string `json:"allowed_origins" validate:"omitempty,dive,url"`

	// Logout Redirect Policy Configuration
	// PostLogoutRedirectURIs: empty array means "clear", nil means "not provided"
	PostLogoutRedirectURIs []string `json:"post_logout_redirect_uris" validate:"omitempty,dive,url"`
	LogoutRedirectPolicy   *string  `json:"logout_redirect_policy" validate:"omitempty,oneof=strict lenient disabled"`
	// DefaultLogoutURI: empty string means "clear", nil means "not provided"
	DefaultLogoutURI    *string `json:"default_logout_uri" validate:"omitempty"`
	AllowWildcardLogout *bool   `json:"allow_wildcard_logout"`

	// Authentication Provider Settings
	// EnabledAuthProviders: array of provider names (email, google, github, microsoft, apple)
	// Empty array means "clear all", nil means "not provided"
	EnabledAuthProviders []string `json:"enabled_auth_providers"`
	AllowEmailSignup     *bool    `json:"allow_email_signup"`
	AllowEmailLogin      *bool    `json:"allow_email_login"`

	// Consent Flow Configuration (pointer: nil = not provided, allows explicit false)
	SkipConsent       *bool `json:"skip_consent"`
	SkipLogoutConsent *bool `json:"skip_logout_consent"`

	// Microsoft OAuth Settings (optional - client-specific credentials)
	MicrosoftOAuthEnabled *bool   `json:"microsoft_oauth_enabled"`
	MicrosoftClientID     *string `json:"microsoft_client_id"`
	MicrosoftClientSecret *string `json:"microsoft_client_secret"`
	MicrosoftTenantID     *string `json:"microsoft_tenant_id"`

	// Apple OAuth Settings (optional - client-specific credentials)
	AppleOAuthEnabled *bool   `json:"apple_oauth_enabled"`
	AppleClientID     *string `json:"apple_client_id"`
	AppleTeamID       *string `json:"apple_team_id"`
	AppleKeyID        *string `json:"apple_key_id"`
	ApplePrivateKey   *string `json:"apple_private_key"`
}

// ClientCredentials represents client ID and secret
type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Sync state constants for SyncStatus.State.
const (
	SyncStateOK      = "ok"      // upstream confirmed the change
	SyncStateFailed  = "failed"  // upstream rejected or unreachable; DB drift
	SyncStateSkipped = "skipped" // operation didn't require an upstream call
)

// SyncStatus describes the outcome of replicating a client-config change to
// the upstream OAuth2/OIDC provider (currently Ory Hydra).
//
// API responses include this so callers can detect drift between Authway's
// database and Hydra without scraping logs. The previous behavior was to
// log a warning and return 200 OK regardless — see the
// `hydra-sync-silent-failure` issue for the AllDot incident this fixes.
type SyncStatus struct {
	State string `json:"state"`           // "ok", "failed", "skipped"
	Error string `json:"error,omitempty"` // human-readable upstream error
}

// OK reports whether the sync completed successfully (or wasn't needed).
func (s SyncStatus) OK() bool {
	return s.State == SyncStateOK || s.State == SyncStateSkipped
}
