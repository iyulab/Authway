package user

import (
	"testing"

	"authway/src/server/pkg/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTenantTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate both tenants and users
	err = db.AutoMigrate(&tenant.Tenant{}, &User{})
	require.NoError(t, err)

	return db
}

func TestService_TenantIsolation(t *testing.T) {
	db := setupTenantTestDB(t)
	logger := zaptest.NewLogger(t)
	userService := NewService(db, logger)

	// Create two tenants
	tenant1 := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Tenant 1",
		Slug:   "tenant-1",
		Active: true,
	}
	tenant2 := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Tenant 2",
		Slug:   "tenant-2",
		Active: true,
	}
	require.NoError(t, db.Create(tenant1).Error)
	require.NoError(t, db.Create(tenant2).Error)

	// Create user in tenant 1
	user1, err := userService.Create(tenant1.ID, &CreateUserRequest{
		Email:    "user@example.com", // Same email!
		Password: "password123",
		Name:     "User 1",
	})
	require.NoError(t, err)
	assert.Equal(t, tenant1.ID, user1.TenantID)

	// Create user with same email in tenant 2 (should succeed - tenant isolation)
	user2, err := userService.Create(tenant2.ID, &CreateUserRequest{
		Email:    "user@example.com", // Same email!
		Password: "password456",
		Name:     "User 2",
	})
	require.NoError(t, err)
	assert.Equal(t, tenant2.ID, user2.TenantID)

	// Verify both users exist with same email but different tenants
	assert.NotEqual(t, user1.ID, user2.ID)
	assert.Equal(t, user1.Email, user2.Email)
	assert.NotEqual(t, user1.TenantID, user2.TenantID)

	// Get by email and tenant should return correct user
	foundUser1, err := userService.GetByEmailAndTenant(tenant1.ID, "user@example.com")
	require.NoError(t, err)
	assert.Equal(t, user1.ID, foundUser1.ID)
	assert.Equal(t, tenant1.ID, foundUser1.TenantID)

	foundUser2, err := userService.GetByEmailAndTenant(tenant2.ID, "user@example.com")
	require.NoError(t, err)
	assert.Equal(t, user2.ID, foundUser2.ID)
	assert.Equal(t, tenant2.ID, foundUser2.TenantID)

	// Try to create duplicate email within same tenant (should fail)
	_, err = userService.Create(tenant1.ID, &CreateUserRequest{
		Email:    "user@example.com",
		Password: "password789",
		Name:     "User 3",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user with email")
}

func TestService_GetByTenant(t *testing.T) {
	db := setupTenantTestDB(t)
	logger := zaptest.NewLogger(t)
	userService := NewService(db, logger)

	// Create two tenants
	tenant1 := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Tenant 1",
		Slug:   "tenant-1",
		Active: true,
	}
	tenant2 := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Tenant 2",
		Slug:   "tenant-2",
		Active: true,
	}
	require.NoError(t, db.Create(tenant1).Error)
	require.NoError(t, db.Create(tenant2).Error)

	// Create 3 users in tenant 1
	for i := 1; i <= 3; i++ {
		_, err := userService.Create(tenant1.ID, &CreateUserRequest{
			Email:    "user" + string(rune('0'+i)) + "@tenant1.com",
			Password: "password123",
			Name:     "User " + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	// Create 2 users in tenant 2
	for i := 1; i <= 2; i++ {
		_, err := userService.Create(tenant2.ID, &CreateUserRequest{
			Email:    "user" + string(rune('0'+i)) + "@tenant2.com",
			Password: "password123",
			Name:     "User " + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	// Get users by tenant 1
	tenant1Users, total1, err := userService.GetByTenant(tenant1.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, tenant1Users, 3)
	assert.Equal(t, int64(3), total1)

	// Verify all users belong to tenant 1
	for _, user := range tenant1Users {
		assert.Equal(t, tenant1.ID, user.TenantID)
	}

	// Get users by tenant 2
	tenant2Users, total2, err := userService.GetByTenant(tenant2.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, tenant2Users, 2)
	assert.Equal(t, int64(2), total2)

	// Verify all users belong to tenant 2
	for _, user := range tenant2Users {
		assert.Equal(t, tenant2.ID, user.TenantID)
	}
}

func TestService_GetByEmailAndTenant(t *testing.T) {
	db := setupTenantTestDB(t)
	logger := zaptest.NewLogger(t)
	userService := NewService(db, logger)

	// Create tenant
	testTenant := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Test Tenant",
		Slug:   "test-tenant",
		Active: true,
	}
	require.NoError(t, db.Create(testTenant).Error)

	// Create user
	testUser, err := userService.Create(testTenant.ID, &CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		tenantID    uuid.UUID
		email       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by email and tenant",
			tenantID:    testTenant.ID,
			email:       testUser.Email,
			expectError: false,
		},
		{
			name:        "wrong tenant ID",
			tenantID:    uuid.New(),
			email:       testUser.Email,
			expectError: true,
			errorMsg:    "user not found",
		},
		{
			name:        "wrong email",
			tenantID:    testTenant.ID,
			email:       "nonexistent@example.com",
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := userService.GetByEmailAndTenant(tt.tenantID, tt.email)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.email, user.Email)
				assert.Equal(t, tt.tenantID, user.TenantID)
				assert.Equal(t, testUser.ID, user.ID)
			}
		})
	}
}

func TestService_UpdatePassword(t *testing.T) {
	db := setupTenantTestDB(t)
	logger := zaptest.NewLogger(t)
	userService := NewService(db, logger)

	// Create tenant
	testTenant := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Test Tenant",
		Slug:   "test-tenant",
		Active: true,
	}
	require.NoError(t, db.Create(testTenant).Error)

	// Create user
	testUser, err := userService.Create(testTenant.ID, &CreateUserRequest{
		Email:    "test@example.com",
		Password: "oldpassword",
		Name:     "Test User",
	})
	require.NoError(t, err)

	// Update password
	newPassword := "newpassword123"
	err = userService.UpdatePassword(testUser.ID, newPassword)
	assert.NoError(t, err)

	// Verify new password works
	updatedUser, err := userService.GetByID(testUser.ID)
	require.NoError(t, err)
	assert.True(t, userService.VerifyPassword(updatedUser, newPassword))
	assert.False(t, userService.VerifyPassword(updatedUser, "oldpassword"))
}

func TestService_UpdateEmailVerified(t *testing.T) {
	db := setupTenantTestDB(t)
	logger := zaptest.NewLogger(t)
	userService := NewService(db, logger)

	// Create tenant
	testTenant := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Test Tenant",
		Slug:   "test-tenant",
		Active: true,
	}
	require.NoError(t, db.Create(testTenant).Error)

	// Create user
	testUser, err := userService.Create(testTenant.ID, &CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
	})
	require.NoError(t, err)
	assert.False(t, testUser.EmailVerified)

	// Update email verified status
	err = userService.UpdateEmailVerified(testUser.ID, true)
	assert.NoError(t, err)

	// Verify email verified is updated
	updatedUser, err := userService.GetByID(testUser.ID)
	require.NoError(t, err)
	assert.True(t, updatedUser.EmailVerified)
}

func TestService_CrossTenantEmailUniqueness(t *testing.T) {
	db := setupTenantTestDB(t)
	logger := zaptest.NewLogger(t)
	userService := NewService(db, logger)

	// Create two tenants
	tenant1 := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Tenant 1",
		Slug:   "tenant-1",
		Active: true,
	}
	tenant2 := &tenant.Tenant{
		ID:     uuid.New(),
		Name:   "Tenant 2",
		Slug:   "tenant-2",
		Active: true,
	}
	require.NoError(t, db.Create(tenant1).Error)
	require.NoError(t, db.Create(tenant2).Error)

	email := "admin@example.com"

	// Create user with email in tenant 1
	user1, err := userService.Create(tenant1.ID, &CreateUserRequest{
		Email:    email,
		Password: "password123",
		Name:     "Admin 1",
	})
	require.NoError(t, err)

	// Create user with same email in tenant 2 (should succeed)
	user2, err := userService.Create(tenant2.ID, &CreateUserRequest{
		Email:    email,
		Password: "password456",
		Name:     "Admin 2",
	})
	require.NoError(t, err)

	// Verify they are different users
	assert.NotEqual(t, user1.ID, user2.ID)
	assert.Equal(t, email, user1.Email)
	assert.Equal(t, email, user2.Email)
	assert.Equal(t, tenant1.ID, user1.TenantID)
	assert.Equal(t, tenant2.ID, user2.TenantID)

	// Query by email should find both users
	var users []User
	db.Where("email = ?", email).Find(&users)
	assert.Len(t, users, 2)
}
