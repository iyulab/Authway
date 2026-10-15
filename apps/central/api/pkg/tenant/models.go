package tenant

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Tenant represents an isolation unit in the system
// Each tenant has independent users and applications
// SSO is automatic within a tenant, isolated between tenants
type Tenant struct {
	ID           uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	Name         string         `json:"name" gorm:"not null"`
	Slug         string         `json:"slug" gorm:"uniqueIndex;not null"`
	Description  string         `json:"description"`
	Settings     TenantSettings `json:"settings" gorm:"type:jsonb"`
	Logo         string         `json:"logo"`
	PrimaryColor string         `json:"primary_color" gorm:"default:#4F46E5"`
	Active       bool           `json:"active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TenantSettings contains tenant-specific configuration
type TenantSettings struct {
	// PasswordMinLength is the least length of a password a user of the
	// tenant may set, when it asks for more than every tenant gets (8).
	// Zero means no more than that.
	PasswordMinLength int `json:"password_min_length" validate:"omitempty,min=8,max=72"`
	// SessionTimeout is how many minutes a sign-in session of the tenant's
	// users lasts. Zero means the default; the most a session can last is 30
	// days (43200 minutes).
	SessionTimeout int `json:"session_timeout" validate:"min=0,max=43200"`
	// AllowedDomains, when it names any, are the only email domains an
	// account of the tenant can be created for — by invitation or by a first
	// sign-in. Accounts that already exist are not affected. See AllowsAddress.
	AllowedDomains []string `json:"allowed_domains" validate:"omitempty,dive,fqdn"`
	// SignupMode governs whether a first-time social/magic-link sign-in may
	// auto-provision an account for this tenant. Checked by
	// invitation.Gate.MayProvision, whitelist-style: only the literal value
	// SignupModeOpen relaxes the invite-only default, so a missing key
	// (every tenant created before this field existed), an empty string, or
	// any unrecognized value all resolve to the safe invite-only behavior
	// with no separate validation required.
	SignupMode string `json:"signup_mode"`
}

const (
	// SignupModeInviteOnly is the default: a first-time sign-in may only
	// create an account for an address that already holds a valid
	// invitation into this tenant.
	SignupModeInviteOnly = "invite_only"
	// SignupModeOpen allows a first-time social/magic-link sign-in to
	// auto-provision an account for any address, bypassing the invitation
	// check — for tenants running a public, self-service signup flow.
	SignupModeOpen = "open"
)

// Scan implements sql.Scanner for TenantSettings (JSONB support)
func (s *TenantSettings) Scan(value any) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to unmarshal JSONB value")
	}

	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer for TenantSettings (JSONB support)
func (s TenantSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// TableName specifies the table name for Tenant model
func (Tenant) TableName() string {
	return "tenants"
}

// AllowsAddress reports whether an account may be created for email under
// AllowedDomains: any address when no domain is named, otherwise one whose
// domain is one of them exactly — a subdomain is a different domain.
func (s TenantSettings) AllowsAddress(email string) bool {
	restricted := false
	at := strings.LastIndex(email, "@")
	for _, allowed := range s.AllowedDomains {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		restricted = true
		if at >= 0 && strings.EqualFold(email[at+1:], allowed) {
			return true
		}
	}
	return !restricted
}

// BeforeCreate sets UUID if not provided
func (t *Tenant) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}

	// Set default settings if not provided
	if t.Settings.PasswordMinLength == 0 {
		t.Settings.PasswordMinLength = 8
	}
	if t.Settings.SessionTimeout == 0 {
		t.Settings.SessionTimeout = 60
	}

	return nil
}

// PublicTenant returns tenant data safe for public consumption. Settings
// carries nothing sensitive (no secrets — verification/password/session/
// domain/signup policy only), so it is included in full rather than
// re-filtered field by field.
type PublicTenant struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Description  string         `json:"description"`
	Settings     TenantSettings `json:"settings"`
	Logo         string         `json:"logo"`
	PrimaryColor string         `json:"primary_color"`
	Active       bool           `json:"active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// ToPublic converts Tenant to PublicTenant
func (t *Tenant) ToPublic() PublicTenant {
	return PublicTenant{
		ID:           t.ID,
		Name:         t.Name,
		Slug:         t.Slug,
		Description:  t.Description,
		Settings:     t.Settings,
		Logo:         t.Logo,
		PrimaryColor: t.PrimaryColor,
		Active:       t.Active,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

// CreateTenantRequest represents the request to create a new tenant
type CreateTenantRequest struct {
	Name         string         `json:"name" validate:"required,min=2,max=255"`
	Slug         string         `json:"slug" validate:"required,min=2,max=100"`
	Description  string         `json:"description" validate:"max=1000"`
	Settings     TenantSettings `json:"settings"`
	Logo         string         `json:"logo" validate:"omitempty,url"`
	PrimaryColor string         `json:"primary_color" validate:"omitempty,hexcolor"`
}

// UpdateTenantRequest represents the request to update a tenant
type UpdateTenantRequest struct {
	Name string `json:"name" validate:"omitempty,min=2,max=255"`
	// Description changes only when named; an empty string clears it.
	Description  *string         `json:"description" validate:"omitempty,max=1000"`
	Settings     *TenantSettings `json:"settings"`
	Logo         string          `json:"logo" validate:"omitempty,url"`
	PrimaryColor string          `json:"primary_color" validate:"omitempty,hexcolor"`
	Active       *bool           `json:"active"` // Pointer to allow explicit false
}

// DefaultTenantID is the UUID of the default tenant
var DefaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// IsDefaultTenant checks if this is the default tenant
func (t *Tenant) IsDefaultTenant() bool {
	return t.ID == DefaultTenantID || t.Slug == "default"
}
