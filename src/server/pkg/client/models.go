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

		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// CreateClientRequest represents the request to create a new OAuth client
type CreateClientRequest struct {
	TenantID     string   `json:"tenant_id" validate:"required,uuid"` // Required!
	ClientID     string   `json:"client_id"`                          // Optional: if provided, use fixed client_id
	ClientSecret string   `json:"client_secret"`                      // Optional: if provided, use fixed client_secret
	Name         string   `json:"name" validate:"required"`
	Description  string   `json:"description"`
	Website      string   `json:"website" validate:"omitempty,url"`
	Logo         string   `json:"logo" validate:"omitempty,url"`
	RedirectURIs []string `json:"redirect_uris" validate:"required,min=1,dive,url"`
	GrantTypes   []string `json:"grant_types" validate:"required,min=1"`
	Scopes       []string `json:"scopes" validate:"required,min=1"`
	Public       bool     `json:"public"`

	// Google OAuth Settings (optional)
	GoogleOAuthEnabled bool   `json:"google_oauth_enabled"`
	GoogleClientID     string `json:"google_client_id" validate:"required_with=GoogleOAuthEnabled"`
	GoogleClientSecret string `json:"google_client_secret" validate:"required_with=GoogleOAuthEnabled"`
	GoogleRedirectURI  string `json:"google_redirect_uri" validate:"required_with=GoogleOAuthEnabled,omitempty,url"`

	// GitHub OAuth Settings (optional)
	GithubOAuthEnabled bool   `json:"github_oauth_enabled"`
	GithubClientID     string `json:"github_client_id" validate:"required_with=GithubOAuthEnabled"`
	GithubClientSecret string `json:"github_client_secret" validate:"required_with=GithubOAuthEnabled"`
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
	Active       *bool    `json:"active"` // Pointer to allow explicit false

	// Google OAuth Settings (optional)
	GoogleOAuthEnabled *bool   `json:"google_oauth_enabled"` // Pointer to allow explicit false
	GoogleClientID     *string `json:"google_client_id"`
	GoogleClientSecret *string `json:"google_client_secret"`
	GoogleRedirectURI  *string `json:"google_redirect_uri" validate:"omitempty,url"`
}

// ClientCredentials represents client ID and secret
type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}
