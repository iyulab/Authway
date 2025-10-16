package tenant

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
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
	Active       bool           `json:"active" gorm:"default:true"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TenantSettings contains tenant-specific configuration
type TenantSettings struct {
	RequireEmailVerification bool     `json:"require_email_verification"`
	PasswordMinLength        int      `json:"password_min_length"`
	SessionTimeout           int      `json:"session_timeout"` // in minutes
	AllowedDomains           []string `json:"allowed_domains"`
}

// Scan implements sql.Scanner for TenantSettings (JSONB support)
func (s *TenantSettings) Scan(value interface{}) error {
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

// PublicTenant returns tenant data safe for public consumption
type PublicTenant struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description"`
	Logo         string    `json:"logo"`
	PrimaryColor string    `json:"primary_color"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ToPublic converts Tenant to PublicTenant
func (t *Tenant) ToPublic() PublicTenant {
	return PublicTenant{
		ID:           t.ID,
		Name:         t.Name,
		Slug:         t.Slug,
		Description:  t.Description,
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
	Name         string          `json:"name" validate:"omitempty,min=2,max=255"`
	Description  string          `json:"description" validate:"max=1000"`
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
