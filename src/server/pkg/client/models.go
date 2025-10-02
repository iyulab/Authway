package client

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Client represents an OAuth 2.0 client application
type Client struct {
	ID           uuid.UUID      `json:"id" gorm:"type:uuid;primary_key"`
	ClientID     string         `json:"client_id" gorm:"uniqueIndex;not null"`
	ClientSecret string         `json:"-" gorm:"not null"` // Never include in JSON responses
	Name         string         `json:"name" gorm:"not null"`
	Description  string         `json:"description"`
	Website      string         `json:"website"`
	Logo         string         `json:"logo"`
	RedirectURIs pq.StringArray `json:"redirect_uris" gorm:"type:text[]"`
	GrantTypes   pq.StringArray `json:"grant_types" gorm:"type:text[]"`
	Scopes       pq.StringArray `json:"scopes" gorm:"type:text[]"`
	Public       bool           `json:"public" gorm:"default:false"` // Public clients (mobile apps, SPAs)
	Active       bool           `json:"active" gorm:"default:true"`

	// Google OAuth Settings (optional - if null, uses central Authway settings)
	GoogleOAuthEnabled bool    `json:"google_oauth_enabled" gorm:"default:false"`
	GoogleClientID     *string `json:"-" gorm:"null"` // Never include in JSON responses
	GoogleClientSecret *string `json:"-" gorm:"null"` // Never include in JSON responses
	GoogleRedirectURI  *string `json:"google_redirect_uri" gorm:"null"`

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

	// Google OAuth Settings (public fields only)
	GoogleOAuthEnabled bool    `json:"google_oauth_enabled"`
	GoogleRedirectURI  *string `json:"google_redirect_uri"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToPublic converts Client to PublicClient
func (c *Client) ToPublic() PublicClient {
	return PublicClient{
		ID:           c.ID,
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

		// Google OAuth public fields
		GoogleOAuthEnabled: c.GoogleOAuthEnabled,
		GoogleRedirectURI:  c.GoogleRedirectURI,

		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// CreateClientRequest represents the request to create a new OAuth client
type CreateClientRequest struct {
	Name         string   `json:"name" validate:"required"`
	Description  string   `json:"description"`
	Website      string   `json:"website" validate:"url"`
	Logo         string   `json:"logo" validate:"url"`
	RedirectURIs []string `json:"redirect_uris" validate:"required,min=1,dive,url"`
	GrantTypes   []string `json:"grant_types" validate:"required,min=1"`
	Scopes       []string `json:"scopes" validate:"required,min=1"`
	Public       bool     `json:"public"`

	// Google OAuth Settings (optional)
	GoogleOAuthEnabled bool   `json:"google_oauth_enabled"`
	GoogleClientID     string `json:"google_client_id" validate:"required_with=GoogleOAuthEnabled"`
	GoogleClientSecret string `json:"google_client_secret" validate:"required_with=GoogleOAuthEnabled"`
	GoogleRedirectURI  string `json:"google_redirect_uri" validate:"required_with=GoogleOAuthEnabled,omitempty,url"`
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
