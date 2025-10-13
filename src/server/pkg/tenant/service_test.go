package tenant

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate the schema
	err = db.AutoMigrate(&Tenant{})
	require.NoError(t, err)

	return db
}

func TestService_Create(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	tests := []struct {
		name        string
		request     *CreateTenantRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful tenant creation",
			request: &CreateTenantRequest{
				Name:        "Test Tenant",
				Slug:        "test-tenant",
				Description: "Test tenant description",
			},
			expectError: false,
		},
		{
			name: "duplicate slug error",
			request: &CreateTenantRequest{
				Name:        "Another Tenant",
				Slug:        "test-tenant", // Same as first test
				Description: "Another description",
			},
			expectError: true,
			errorMsg:    "tenant with slug test-tenant already exists",
		},
		{
			name: "tenant with custom settings",
			request: &CreateTenantRequest{
				Name:         "Custom Tenant",
				Slug:         "custom-tenant",
				PrimaryColor: "#FF5733",
				Logo:         "https://example.com/logo.png",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant, err := service.Create(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, tenant)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.NotEmpty(t, tenant.ID)
				assert.Equal(t, tt.request.Name, tenant.Name)
				assert.Equal(t, tt.request.Slug, tenant.Slug)
				assert.True(t, tenant.Active)

				if tt.request.PrimaryColor != "" {
					assert.Equal(t, tt.request.PrimaryColor, *tenant.PrimaryColor)
				}
				if tt.request.Logo != "" {
					assert.Equal(t, tt.request.Logo, *tenant.Logo)
				}
			}
		})
	}
}

func TestService_GetByID(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test tenant
	testTenant, err := service.Create(&CreateTenantRequest{
		Name: "Test Tenant",
		Slug: "test-tenant",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		tenantID    uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by ID",
			tenantID:    testTenant.ID,
			expectError: false,
		},
		{
			name:        "tenant not found",
			tenantID:    uuid.New(),
			expectError: true,
			errorMsg:    "tenant not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant, err := service.GetByID(tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, tenant)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.Equal(t, tt.tenantID, tenant.ID)
				assert.Equal(t, testTenant.Name, tenant.Name)
			}
		})
	}
}

func TestService_GetBySlug(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test tenant
	testTenant, err := service.Create(&CreateTenantRequest{
		Name: "Test Tenant",
		Slug: "test-tenant",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		slug        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by slug",
			slug:        testTenant.Slug,
			expectError: false,
		},
		{
			name:        "tenant not found",
			slug:        "nonexistent-slug",
			expectError: true,
			errorMsg:    "tenant not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant, err := service.GetBySlug(tt.slug)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, tenant)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.Equal(t, tt.slug, tenant.Slug)
				assert.Equal(t, testTenant.ID, tenant.ID)
			}
		})
	}
}

func TestService_List(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create multiple test tenants
	for i := 1; i <= 5; i++ {
		_, err := service.Create(&CreateTenantRequest{
			Name: string(rune('A'+i-1)) + " Tenant",
			Slug: string(rune('a'+i-1)) + "-tenant",
		})
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		limit         int
		offset        int
		expectedLen   int
		expectedTotal int64
	}{
		{
			name:          "get all tenants",
			limit:         10,
			offset:        0,
			expectedLen:   5,
			expectedTotal: 5,
		},
		{
			name:          "paginated results - first page",
			limit:         2,
			offset:        0,
			expectedLen:   2,
			expectedTotal: 5,
		},
		{
			name:          "paginated results - second page",
			limit:         2,
			offset:        2,
			expectedLen:   2,
			expectedTotal: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenants, total, err := service.List(tt.limit, tt.offset)

			assert.NoError(t, err)
			assert.Len(t, tenants, tt.expectedLen)
			assert.Equal(t, tt.expectedTotal, total)

			// Verify tenants are not nil
			for _, tenant := range tenants {
				assert.NotNil(t, tenant)
				assert.NotEmpty(t, tenant.ID)
				assert.NotEmpty(t, tenant.Name)
				assert.NotEmpty(t, tenant.Slug)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test tenant
	testTenant, err := service.Create(&CreateTenantRequest{
		Name: "Test Tenant",
		Slug: "test-tenant",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		tenantID    uuid.UUID
		request     *UpdateTenantRequest
		expectError bool
		errorMsg    string
	}{
		{
			name:     "successful update",
			tenantID: testTenant.ID,
			request: &UpdateTenantRequest{
				Name:         "Updated Tenant",
				Description:  "Updated description",
				PrimaryColor: "#FF5733",
			},
			expectError: false,
		},
		{
			name:     "partial update",
			tenantID: testTenant.ID,
			request: &UpdateTenantRequest{
				Name: "Partially Updated",
			},
			expectError: false,
		},
		{
			name:        "tenant not found",
			tenantID:    uuid.New(),
			request:     &UpdateTenantRequest{Name: "Test"},
			expectError: true,
			errorMsg:    "tenant not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant, err := service.Update(tt.tenantID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, tenant)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.Equal(t, tt.tenantID, tenant.ID)

				if tt.request.Name != "" {
					assert.Equal(t, tt.request.Name, tenant.Name)
				}
				if tt.request.Description != "" {
					assert.Equal(t, tt.request.Description, *tenant.Description)
				}
				if tt.request.PrimaryColor != "" {
					assert.Equal(t, tt.request.PrimaryColor, *tenant.PrimaryColor)
				}
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test tenant
	testTenant, err := service.Create(&CreateTenantRequest{
		Name: "Test Tenant",
		Slug: "test-tenant",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		tenantID    uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful delete",
			tenantID:    testTenant.ID,
			expectError: false,
		},
		{
			name:        "tenant not found",
			tenantID:    uuid.New(),
			expectError: true,
			errorMsg:    "tenant not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.Delete(tt.tenantID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)

				// Verify tenant is deleted
				_, getErr := service.GetByID(tt.tenantID)
				assert.Error(t, getErr)
				assert.Contains(t, getErr.Error(), "tenant not found")
			}
		})
	}
}

func TestService_EnsureDefaultTenant(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// First call should create default tenant
	err := service.EnsureDefaultTenant()
	assert.NoError(t, err)

	// Verify default tenant exists
	tenant, err := service.GetBySlug("default")
	assert.NoError(t, err)
	assert.NotNil(t, tenant)
	assert.Equal(t, "Default Tenant", tenant.Name)
	assert.Equal(t, "default", tenant.Slug)

	// Second call should not error (idempotent)
	err = service.EnsureDefaultTenant()
	assert.NoError(t, err)

	// Verify still only one default tenant
	var count int64
	db.Model(&Tenant{}).Where("slug = ?", "default").Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestService_CreateSingleTenant(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	tests := []struct {
		name        string
		tenantName  string
		tenantSlug  string
		expectError bool
	}{
		{
			name:        "successful single tenant creation",
			tenantName:  "App A",
			tenantSlug:  "app-a",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant, err := service.CreateSingleTenant(tt.tenantName, tt.tenantSlug)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.Equal(t, tt.tenantName, tenant.Name)
				assert.Equal(t, tt.tenantSlug, tenant.Slug)
				assert.True(t, tenant.Active)

				// Verify it's the only tenant
				var count int64
				db.Model(&Tenant{}).Count(&count)
				assert.Equal(t, int64(1), count)
			}
		})
	}
}
