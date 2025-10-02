package auth

import (
	"testing"

	"authway/src/server/internal/config"
	"authway/src/server/pkg/user"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate the schema
	err = db.AutoMigrate(&user.User{})
	require.NoError(t, err)

	return db
}

func setupTestService(t *testing.T) (*service, *gorm.DB) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)

	// Mock Redis client (can be nil for basic tests)
	var redisClient *redis.Client

	// Mock config
	cfg := &config.Config{}

	svc := &service{
		db:     db,
		redis:  redisClient,
		config: cfg,
		logger: logger,
	}

	return svc, db
}

func TestNewService(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	var redisClient *redis.Client
	cfg := &config.Config{}

	service := NewService(db, redisClient, cfg, logger)

	assert.NotNil(t, service)
	assert.IsType(t, &service{}, service)
}

func TestService_Authenticate(t *testing.T) {
	svc, db := setupTestService(t)

	// Create test user with hashed password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	testUser := &user.User{
		Email:        "test@example.com",
		PasswordHash: &hashedPassword,
		FirstName:    stringPtr("John"),
		LastName:     stringPtr("Doe"),
		Active:       true,
	}

	err = db.Create(testUser).Error
	require.NoError(t, err)

	tests := []struct {
		name        string
		email       string
		password    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful authentication - user exists",
			email:       "test@example.com",
			password:    "password123",
			expectError: false,
		},
		{
			name:        "user not found",
			email:       "nonexistent@example.com",
			password:    "password123",
			expectError: true,
			errorMsg:    "user not found",
		},
		{
			name:        "empty email",
			email:       "",
			password:    "password123",
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := svc.Authenticate(tt.email, tt.password)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.email, user.Email)
				assert.NotEmpty(t, user.ID)
			}
		})
	}
}

func TestService_Authenticate_DatabaseError(t *testing.T) {
	svc, db := setupTestService(t)

	// Close the database to simulate a database error
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	user, err := svc.Authenticate("test@example.com", "password")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user")
	assert.Nil(t, user)
}

func TestService_Authenticate_WithInactiveUser(t *testing.T) {
	svc, db := setupTestService(t)

	// Create inactive user
	testUser := &user.User{
		Email:     "inactive@example.com",
		FirstName: stringPtr("Inactive"),
		LastName:  stringPtr("User"),
		Active:    false, // User is inactive
	}

	err := db.Create(testUser).Error
	require.NoError(t, err)

	// The current implementation doesn't check Active status
	// This test documents the current behavior
	user, err := svc.Authenticate("inactive@example.com", "password")

	assert.NoError(t, err) // Current implementation doesn't check active status
	assert.NotNil(t, user)
	assert.False(t, user.Active)
}

func TestService_Authenticate_WithUnverifiedEmail(t *testing.T) {
	svc, db := setupTestService(t)

	// Create user with unverified email
	testUser := &user.User{
		Email:         "unverified@example.com",
		FirstName:     stringPtr("Unverified"),
		LastName:      stringPtr("User"),
		Active:        true,
		EmailVerified: false, // Email not verified
	}

	err := db.Create(testUser).Error
	require.NoError(t, err)

	// The current implementation doesn't check email verification
	// This test documents the current behavior
	user, err := svc.Authenticate("unverified@example.com", "password")

	assert.NoError(t, err) // Current implementation doesn't check email verification
	assert.NotNil(t, user)
	assert.False(t, user.EmailVerified)
}

func TestService_Authenticate_CaseInsensitiveEmail(t *testing.T) {
	svc, db := setupTestService(t)

	// Create user with lowercase email
	testUser := &user.User{
		Email:     "test@example.com",
		FirstName: stringPtr("Test"),
		LastName:  stringPtr("User"),
		Active:    true,
	}

	err := db.Create(testUser).Error
	require.NoError(t, err)

	tests := []struct {
		name  string
		email string
	}{
		{
			name:  "uppercase email",
			email: "TEST@EXAMPLE.COM",
		},
		{
			name:  "mixed case email",
			email: "Test@Example.Com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Current implementation does exact match
			// This documents the behavior - may need case-insensitive matching
			user, err := svc.Authenticate(tt.email, "password")

			// Current implementation will not find the user (case sensitive)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "user not found")
			assert.Nil(t, user)
		})
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
