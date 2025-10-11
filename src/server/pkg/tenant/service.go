package tenant

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Service handles tenant operations
type Service struct {
	db *gorm.DB
}

// NewService creates a new tenant service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateTenant creates a new tenant
func (s *Service) CreateTenant(req CreateTenantRequest) (*Tenant, error) {
	// Check if slug already exists
	var existing Tenant
	if err := s.db.Where("slug = ?", req.Slug).First(&existing).Error; err == nil {
		return nil, ErrDuplicateSlug
	}

	tenant := &Tenant{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Settings:     req.Settings,
		Logo:         req.Logo,
		PrimaryColor: req.PrimaryColor,
		Active:       true,
	}

	if err := s.db.Create(tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	return tenant, nil
}

// GetTenantBySlug retrieves tenant by slug
func (s *Service) GetTenantBySlug(slug string) (*Tenant, error) {
	var tenant Tenant
	if err := s.db.Where("slug = ? AND deleted_at IS NULL", slug).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// GetTenantByID retrieves tenant by ID
func (s *Service) GetTenantByID(id uuid.UUID) (*Tenant, error) {
	var tenant Tenant
	if err := s.db.Where("id = ? AND deleted_at IS NULL", id).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// ListTenants lists all active tenants
func (s *Service) ListTenants() ([]Tenant, error) {
	var tenants []Tenant
	if err := s.db.Where("deleted_at IS NULL").Order("created_at DESC").Find(&tenants).Error; err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	return tenants, nil
}

// UpdateTenant updates tenant information
func (s *Service) UpdateTenant(id uuid.UUID, req UpdateTenantRequest) (*Tenant, error) {
	tenant, err := s.GetTenantByID(id)
	if err != nil {
		return nil, err
	}

	// Prevent updating default tenant slug or deletion
	if tenant.IsDefaultTenant() && req.Active != nil && !*req.Active {
		return nil, ErrCannotDeactivateDefault
	}

	// Update fields
	if req.Name != "" {
		tenant.Name = req.Name
	}
	if req.Description != "" {
		tenant.Description = req.Description
	}
	if req.Settings != nil {
		tenant.Settings = *req.Settings
	}
	if req.Logo != "" {
		tenant.Logo = req.Logo
	}
	if req.PrimaryColor != "" {
		tenant.PrimaryColor = req.PrimaryColor
	}
	if req.Active != nil {
		tenant.Active = *req.Active
	}

	if err := s.db.Save(tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	return tenant, nil
}

// DeleteTenant soft deletes a tenant
func (s *Service) DeleteTenant(id uuid.UUID) error {
	tenant, err := s.GetTenantByID(id)
	if err != nil {
		return err
	}

	// Prevent deleting default tenant
	if tenant.IsDefaultTenant() {
		return ErrCannotDeleteDefault
	}

	// Check if tenant has active (non-deleted) users
	var userCount int64
	if err := s.db.Table("users").Where("tenant_id = ? AND deleted_at IS NULL", id).Count(&userCount).Error; err != nil {
		return fmt.Errorf("failed to check user count: %w", err)
	}
	if userCount > 0 {
		return ErrHasUsers
	}

	// Check if tenant has active (non-deleted) clients
	var clientCount int64
	if err := s.db.Table("clients").Where("tenant_id = ? AND deleted_at IS NULL", id).Count(&clientCount).Error; err != nil {
		return fmt.Errorf("failed to check client count: %w", err)
	}
	if clientCount > 0 {
		return ErrHasClients
	}

	// Soft delete
	if err := s.db.Delete(tenant).Error; err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	return nil
}

// GetDefaultTenant retrieves the default tenant
func (s *Service) GetDefaultTenant() (*Tenant, error) {
	return s.GetTenantByID(DefaultTenantID)
}

// EnsureDefaultTenant ensures the default tenant exists
func (s *Service) EnsureDefaultTenant() error {
	var tenant Tenant

	// Check by ID first (including soft-deleted)
	err := s.db.Unscoped().Where("id = ?", DefaultTenantID).First(&tenant).Error
	if err == nil {
		// Default tenant exists (active or soft-deleted)
		if tenant.DeletedAt.Valid {
			// Restore soft-deleted default tenant
			return s.db.Model(&tenant).Update("deleted_at", nil).Error
		}
		return nil
	}

	// Check by slug (in case ID is different but slug exists)
	err = s.db.Unscoped().Where("slug = ?", "default").First(&tenant).Error
	if err == nil {
		// Tenant with slug 'default' exists, just return (don't create duplicate)
		if tenant.DeletedAt.Valid {
			return s.db.Model(&tenant).Update("deleted_at", nil).Error
		}
		return nil
	}

	// Both checks failed with non-RecordNotFound error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check default tenant: %w", err)
	}

	// Create default tenant
	defaultTenant := &Tenant{
		ID:          DefaultTenantID,
		Name:        "Default",
		Slug:        "default",
		Description: "Default tenant for backward compatibility and initial setup",
		Settings: TenantSettings{
			RequireEmailVerification: true,
			PasswordMinLength:        8,
			SessionTimeout:           60,
			AllowedDomains:           []string{},
		},
		Active: true,
	}

	if err := s.db.Create(defaultTenant).Error; err != nil {
		return fmt.Errorf("failed to create default tenant: %w", err)
	}

	return nil
}

// CreateSingleTenant creates a single tenant for Single Tenant Mode
func (s *Service) CreateSingleTenant(name, slug string) (*Tenant, error) {
	// Check if tenant already exists
	var existing Tenant
	if err := s.db.Where("slug = ?", slug).First(&existing).Error; err == nil {
		// Tenant already exists, return it
		return &existing, nil
	}

	// Create new tenant
	tenant := &Tenant{
		Name:        name,
		Slug:        slug,
		Description: fmt.Sprintf("Single tenant for %s", name),
		Settings: TenantSettings{
			RequireEmailVerification: true,
			PasswordMinLength:        8,
			SessionTimeout:           60,
			AllowedDomains:           []string{},
		},
		Active: true,
	}

	if err := s.db.Create(tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to create single tenant: %w", err)
	}

	return tenant, nil
}
