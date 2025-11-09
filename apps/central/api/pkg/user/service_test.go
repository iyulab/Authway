package user

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
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
	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	return db
}

func TestService_Create(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	tests := []struct {
		name        string
		request     *CreateUserRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful user creation",
			request: &CreateUserRequest{
				Email:     "test@example.com",
				Password:  "password123",
				FirstName: "John",
				LastName:  "Doe",
			},
			expectError: false,
		},
		{
			name: "user creation without password (social login)",
			request: &CreateUserRequest{
				Email:     "social@example.com",
				FirstName: "Jane",
				LastName:  "Smith",
			},
			expectError: false,
		},
		{
			name: "duplicate email error",
			request: &CreateUserRequest{
				Email:     "test@example.com", // Same as first test
				Password:  "password123",
				FirstName: "Another",
				LastName:  "User",
			},
			expectError: true,
			errorMsg:    "user with email test@example.com already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := service.Create(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.NotEmpty(t, user.ID)
				assert.Equal(t, tt.request.Email, user.Email)
				assert.Equal(t, tt.request.FirstName, *user.FirstName)
				assert.Equal(t, tt.request.LastName, *user.LastName)
				assert.False(t, user.EmailVerified)
				assert.True(t, user.Active)

				if tt.request.Password != "" {
					assert.NotNil(t, user.PasswordHash)
					// Verify password can be verified
					assert.True(t, service.VerifyPassword(user, tt.request.Password))
				} else {
					assert.Nil(t, user.PasswordHash)
				}
			}
		})
	}
}

func TestService_GetByID(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by ID",
			userID:      testUser.ID,
			expectError: false,
		},
		{
			name:        "user not found",
			userID:      uuid.New(),
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := service.GetByID(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.userID, user.ID)
				assert.Equal(t, testUser.Email, user.Email)
			}
		})
	}
}

func TestService_GetByEmail(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		email       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by email",
			email:       testUser.Email,
			expectError: false,
		},
		{
			name:        "user not found",
			email:       "nonexistent@example.com",
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := service.GetByEmail(tt.email)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.email, user.Email)
				assert.Equal(t, testUser.ID, user.ID)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      uuid.UUID
		request     *UpdateUserRequest
		expectError bool
		errorMsg    string
	}{
		{
			name:   "successful update",
			userID: testUser.ID,
			request: &UpdateUserRequest{
				FirstName: "Jane",
				LastName:  "Smith",
				Avatar:    "https://example.com/avatar.jpg",
			},
			expectError: false,
		},
		{
			name:   "partial update",
			userID: testUser.ID,
			request: &UpdateUserRequest{
				FirstName: "Updated",
			},
			expectError: false,
		},
		{
			name:        "user not found",
			userID:      uuid.New(),
			request:     &UpdateUserRequest{FirstName: "Test"},
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := service.Update(tt.userID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.userID, user.ID)

				if tt.request.FirstName != "" {
					assert.Equal(t, tt.request.FirstName, *user.FirstName)
				}
				if tt.request.LastName != "" {
					assert.Equal(t, tt.request.LastName, *user.LastName)
				}
				if tt.request.Avatar != "" {
					assert.Equal(t, tt.request.Avatar, *user.Avatar)
				}
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful delete",
			userID:      testUser.ID,
			expectError: false,
		},
		{
			name:        "user not found",
			userID:      uuid.New(),
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.Delete(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)

				// Verify user is deleted
				_, getErr := service.GetByID(tt.userID)
				assert.Error(t, getErr)
				assert.Contains(t, getErr.Error(), "user not found")
			}
		})
	}
}

func TestService_List(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create multiple test users
	users := make([]*User, 5)
	for i := 0; i < 5; i++ {
		user, err := service.Create(&CreateUserRequest{
			Email:     fmt.Sprintf("user%d@example.com", i),
			Password:  "password123",
			FirstName: fmt.Sprintf("User%d", i),
			LastName:  "Test",
		})
		require.NoError(t, err)
		users[i] = user
	}

	tests := []struct {
		name          string
		limit         int
		offset        int
		expectedLen   int
		expectedTotal int64
	}{
		{
			name:          "get all users",
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
		{
			name:          "paginated results - last page",
			limit:         2,
			offset:        4,
			expectedLen:   1,
			expectedTotal: 5,
		},
		{
			name:          "offset beyond results",
			limit:         10,
			offset:        10,
			expectedLen:   0,
			expectedTotal: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users, total, err := service.List(tt.limit, tt.offset)

			assert.NoError(t, err)
			assert.Len(t, users, tt.expectedLen)
			assert.Equal(t, tt.expectedTotal, total)

			// Verify users are not nil
			for _, user := range users {
				assert.NotNil(t, user)
				assert.NotEmpty(t, user.ID)
				assert.NotEmpty(t, user.Email)
			}
		})
	}
}

func TestService_VerifyPassword(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user with password
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	// Create test user without password (social login)
	socialUser, err := service.Create(&CreateUserRequest{
		Email:     "social@example.com",
		FirstName: "Jane",
		LastName:  "Smith",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		user     *User
		password string
		expected bool
	}{
		{
			name:     "correct password",
			user:     testUser,
			password: "password123",
			expected: true,
		},
		{
			name:     "incorrect password",
			user:     testUser,
			password: "wrongpassword",
			expected: false,
		},
		{
			name:     "user with no password hash",
			user:     socialUser,
			password: "anypassword",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.VerifyPassword(tt.user, tt.password)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestService_ChangePassword(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "oldpassword",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	// Create user without password
	socialUser, err := service.Create(&CreateUserRequest{
		Email:     "social@example.com",
		FirstName: "Jane",
		LastName:  "Smith",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      uuid.UUID
		request     *ChangePasswordRequest
		expectError bool
		errorMsg    string
	}{
		{
			name:   "successful password change",
			userID: testUser.ID,
			request: &ChangePasswordRequest{
				CurrentPassword: "oldpassword",
				NewPassword:     "newpassword123",
			},
			expectError: false,
		},
		{
			name:   "incorrect current password",
			userID: testUser.ID,
			request: &ChangePasswordRequest{
				CurrentPassword: "wrongpassword",
				NewPassword:     "newpassword123",
			},
			expectError: true,
			errorMsg:    "current password is incorrect",
		},
		{
			name:   "user without password hash",
			userID: socialUser.ID,
			request: &ChangePasswordRequest{
				CurrentPassword: "anypassword",
				NewPassword:     "newpassword123",
			},
			expectError: true,
			errorMsg:    "current password is incorrect",
		},
		{
			name:        "user not found",
			userID:      uuid.New(),
			request:     &ChangePasswordRequest{CurrentPassword: "old", NewPassword: "new"},
			expectError: true,
			errorMsg:    "user not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ChangePassword(tt.userID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)

				// Verify new password works
				user, getErr := service.GetByID(tt.userID)
				require.NoError(t, getErr)
				assert.True(t, service.VerifyPassword(user, tt.request.NewPassword))
				assert.False(t, service.VerifyPassword(user, tt.request.CurrentPassword))
			}
		})
	}
}

func TestService_UpdateLastLogin(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test user
	testUser, err := service.Create(&CreateUserRequest{
		Email:     "test@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      uuid.UUID
		expectError bool
	}{
		{
			name:        "successful update last login",
			userID:      testUser.ID,
			expectError: false,
		},
		{
			name:        "user not found - should not error but no rows affected",
			userID:      uuid.New(),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.UpdateLastLogin(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.userID == testUser.ID {
					// Verify last login was updated
					user, getErr := service.GetByID(tt.userID)
					require.NoError(t, getErr)
					assert.NotNil(t, user.LastLoginAt)
				}
			}
		})
	}
}

// Helper function to verify password hashing works correctly
func TestPasswordHashing(t *testing.T) {
	password := "testpassword123"

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	// Verify password
	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	assert.NoError(t, err)

	// Verify wrong password fails
	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte("wrongpassword"))
	assert.Error(t, err)
}
