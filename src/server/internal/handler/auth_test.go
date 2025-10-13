package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"authway/src/server/internal/hydra"
	"authway/src/server/pkg/client"
	"authway/src/server/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// Mock user service
type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) Create(req *user.CreateUserRequest) (*user.User, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) GetByID(id uuid.UUID) (*user.User, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) GetByEmail(email string) (*user.User, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) Update(id uuid.UUID, req *user.UpdateUserRequest) (*user.User, error) {
	args := m.Called(id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) Delete(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockUserService) List(limit, offset int) ([]*user.User, int64, error) {
	args := m.Called(limit, offset)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*user.User), args.Get(1).(int64), args.Error(2)
}

func (m *MockUserService) VerifyPassword(user *user.User, password string) bool {
	args := m.Called(user, password)
	return args.Bool(0)
}

func (m *MockUserService) ChangePassword(id uuid.UUID, req *user.ChangePasswordRequest) error {
	args := m.Called(id, req)
	return args.Error(0)
}

func (m *MockUserService) UpdateLastLogin(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

// Mock client service
type MockClientService struct {
	mock.Mock
}

func (m *MockClientService) Create(tenantID uuid.UUID, req *client.CreateClientRequest) (*client.Client, error) {
	args := m.Called(tenantID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*client.Client), args.Error(1)
}

func (m *MockClientService) GetByID(id uuid.UUID) (*client.Client, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*client.Client), args.Error(1)
}

func (m *MockClientService) GetByClientID(clientID string) (*client.Client, error) {
	args := m.Called(clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*client.Client), args.Error(1)
}

func (m *MockClientService) Update(id uuid.UUID, req *client.UpdateClientRequest) (*client.Client, error) {
	args := m.Called(id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*client.Client), args.Error(1)
}

func (m *MockClientService) Delete(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockClientService) List(tenantID uuid.UUID, limit, offset int) ([]*client.Client, int64, error) {
	args := m.Called(tenantID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*client.Client), args.Get(1).(int64), args.Error(2)
}

// Mock hydra client
type MockHydraClient struct {
	mock.Mock
}

func (m *MockHydraClient) GetLoginRequest(challenge string) (*hydra.LoginRequest, error) {
	args := m.Called(challenge)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.LoginRequest), args.Error(1)
}

func (m *MockHydraClient) AcceptLoginRequest(challenge string, body *hydra.AcceptLoginRequest) (*hydra.LoginResponse, error) {
	args := m.Called(challenge, body)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.LoginResponse), args.Error(1)
}

func (m *MockHydraClient) RejectLoginRequest(challenge string, errorCode, errorDescription string) (*hydra.LoginResponse, error) {
	args := m.Called(challenge, errorCode, errorDescription)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.LoginResponse), args.Error(1)
}

func (m *MockHydraClient) GetConsentRequest(challenge string) (*hydra.ConsentRequest, error) {
	args := m.Called(challenge)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.ConsentRequest), args.Error(1)
}

func (m *MockHydraClient) AcceptConsentRequest(challenge string, body *hydra.AcceptConsentRequest) (*hydra.LoginResponse, error) {
	args := m.Called(challenge, body)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.LoginResponse), args.Error(1)
}

func (m *MockHydraClient) RejectConsentRequest(challenge string, errorCode, errorDescription string) (*hydra.LoginResponse, error) {
	args := m.Called(challenge, errorCode, errorDescription)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.LoginResponse), args.Error(1)
}

func TestNewAuthHandler(t *testing.T) {
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}

	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	assert.NotNil(t, handler)
	assert.Equal(t, mockUserService, handler.userService)
	assert.Equal(t, mockHydraClient, handler.hydraClient)
}

func TestAuthHandler_LoginPage(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Get("/login", handler.LoginPage)

	tests := []struct {
		name           string
		challenge      string
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing challenge parameter",
			challenge:      "",
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "login_challenge parameter is required",
		},
		{
			name:      "hydra client error",
			challenge: "test-challenge",
			setupMocks: func() {
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to get login request",
		},
		{
			name:      "successful login page with skip=false",
			challenge: "test-challenge",
			setupMocks: func() {
				loginReq := &hydra.LoginRequest{
					Challenge:      "test-challenge",
					Skip:           false,
					RequestedScope: []string{"openid", "email"},
					Client: &hydra.OAuth2Client{
						ClientName: "Test App",
					},
				}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
			},
			expectedStatus: 200,
		},
		{
			name:      "successful login page with skip=true",
			challenge: "test-challenge",
			setupMocks: func() {
				loginReq := &hydra.LoginRequest{
					Challenge:      "test-challenge",
					Skip:           true,
					Subject:        "user-123",
					RequestedScope: []string{"openid", "email"},
					Client: &hydra.OAuth2Client{
						ClientName: "Test App",
					},
				}
				loginResponse := &hydra.LoginResponse{
					RedirectTo: "http://example.com/callback",
				}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
				mockHydraClient.On("AcceptLoginRequest", "test-challenge", mock.AnythingOfType("*hydra.AcceptLoginRequest")).
					Return(loginResponse, nil).Once()
			},
			expectedStatus: 302, // Redirect
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			url := "/login"
			if tt.challenge != "" {
				url += "?login_challenge=" + tt.challenge
			}

			req := httptest.NewRequest("GET", url, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_Login(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Post("/login", handler.Login)

	// Create test user with password
	userID := uuid.New()
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	hashedPasswordBytes := hashedPassword
	testUser := &user.User{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: &hashedPasswordBytes,
		FirstName:    stringPtr("John"),
		LastName:     stringPtr("Doe"),
	}

	tests := []struct {
		name           string
		requestBody    LoginRequest
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid request body",
			requestBody:    LoginRequest{},
			setupMocks:     func() {},
			expectedStatus: 400,
		},
		{
			name: "hydra client error on get login request",
			requestBody: LoginRequest{
				Challenge: "test-challenge",
				Email:     "test@example.com",
				Password:  "password123",
			},
			setupMocks: func() {
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to get login request",
		},
		{
			name: "user not found",
			requestBody: LoginRequest{
				Challenge: "test-challenge",
				Email:     "nonexistent@example.com",
				Password:  "password123",
			},
			setupMocks: func() {
				loginReq := &hydra.LoginRequest{Challenge: "test-challenge"}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
				mockUserService.On("GetByEmail", "nonexistent@example.com").
					Return(nil, assert.AnError).Once()
				loginResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/error"}
				mockHydraClient.On("RejectLoginRequest", "test-challenge", "invalid_credentials", "Invalid email or password").
					Return(loginResponse, nil).Once()
			},
			expectedStatus: 200,
			expectedError:  "Invalid email or password",
		},
		{
			name: "invalid password",
			requestBody: LoginRequest{
				Challenge: "test-challenge",
				Email:     "test@example.com",
				Password:  "wrongpassword",
			},
			setupMocks: func() {
				loginReq := &hydra.LoginRequest{Challenge: "test-challenge"}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
				mockUserService.On("GetByEmail", "test@example.com").
					Return(testUser, nil).Once()
				loginResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/error"}
				mockHydraClient.On("RejectLoginRequest", "test-challenge", "invalid_credentials", "Invalid email or password").
					Return(loginResponse, nil).Once()
			},
			expectedStatus: 200,
			expectedError:  "Invalid email or password",
		},
		{
			name: "user with no password hash",
			requestBody: LoginRequest{
				Challenge: "test-challenge",
				Email:     "social@example.com",
				Password:  "password123",
			},
			setupMocks: func() {
				socialUser := &user.User{
					ID:           uuid.New(),
					Email:        "social@example.com",
					PasswordHash: nil,
					FirstName:    stringPtr("Social"),
					LastName:     stringPtr("User"),
				}
				loginReq := &hydra.LoginRequest{Challenge: "test-challenge"}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
				mockUserService.On("GetByEmail", "social@example.com").
					Return(socialUser, nil).Once()
				loginResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/error"}
				mockHydraClient.On("RejectLoginRequest", "test-challenge", "invalid_credentials", "Invalid email or password").
					Return(loginResponse, nil).Once()
			},
			expectedStatus: 200,
			expectedError:  "Invalid email or password",
		},
		{
			name: "successful login without remember",
			requestBody: LoginRequest{
				Challenge: "test-challenge",
				Email:     "test@example.com",
				Password:  "password123",
				Remember:  false,
			},
			setupMocks: func() {
				loginReq := &hydra.LoginRequest{Challenge: "test-challenge"}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
				mockUserService.On("GetByEmail", "test@example.com").
					Return(testUser, nil).Once()
				loginResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/callback"}
				mockHydraClient.On("AcceptLoginRequest", "test-challenge", mock.AnythingOfType("*hydra.AcceptLoginRequest")).
					Return(loginResponse, nil).Once()
			},
			expectedStatus: 200,
		},
		{
			name: "successful login with remember",
			requestBody: LoginRequest{
				Challenge: "test-challenge",
				Email:     "test@example.com",
				Password:  "password123",
				Remember:  true,
			},
			setupMocks: func() {
				loginReq := &hydra.LoginRequest{Challenge: "test-challenge"}
				mockHydraClient.On("GetLoginRequest", "test-challenge").
					Return(loginReq, nil).Once()
				mockUserService.On("GetByEmail", "test@example.com").
					Return(testUser, nil).Once()
				loginResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/callback"}
				mockHydraClient.On("AcceptLoginRequest", "test-challenge", mock.AnythingOfType("*hydra.AcceptLoginRequest")).
					Return(loginResponse, nil).Once()
			},
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_ConsentPage(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Get("/consent", handler.ConsentPage)

	userID := uuid.New()
	testUser := &user.User{
		ID:        userID,
		Email:     "test@example.com",
		FirstName: stringPtr("John"),
		LastName:  stringPtr("Doe"),
	}

	tests := []struct {
		name           string
		challenge      string
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing challenge parameter",
			challenge:      "",
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "consent_challenge parameter is required",
		},
		{
			name:      "hydra client error",
			challenge: "test-challenge",
			setupMocks: func() {
				mockHydraClient.On("GetConsentRequest", "test-challenge").
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to get consent request",
		},
		{
			name:      "invalid user ID in consent request",
			challenge: "test-challenge",
			setupMocks: func() {
				consentReq := &hydra.ConsentRequest{
					Challenge: "test-challenge",
					Subject:   "invalid-uuid",
				}
				mockHydraClient.On("GetConsentRequest", "test-challenge").
					Return(consentReq, nil).Once()
			},
			expectedStatus: 500,
			expectedError:  "Invalid user ID",
		},
		{
			name:      "user not found",
			challenge: "test-challenge",
			setupMocks: func() {
				consentReq := &hydra.ConsentRequest{
					Challenge: "test-challenge",
					Subject:   userID.String(),
				}
				mockHydraClient.On("GetConsentRequest", "test-challenge").
					Return(consentReq, nil).Once()
				mockUserService.On("GetByID", userID).
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to get user information",
		},
		{
			name:      "successful consent page",
			challenge: "test-challenge",
			setupMocks: func() {
				consentReq := &hydra.ConsentRequest{
					Challenge:      "test-challenge",
					Subject:        userID.String(),
					RequestedScope: []string{"openid", "email"},
					Client: &hydra.OAuth2Client{
						ClientName: "Test App",
					},
				}
				mockHydraClient.On("GetConsentRequest", "test-challenge").
					Return(consentReq, nil).Once()
				mockUserService.On("GetByID", userID).
					Return(testUser, nil).Once()
			},
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			url := "/consent"
			if tt.challenge != "" {
				url += "?consent_challenge=" + tt.challenge
			}

			req := httptest.NewRequest("GET", url, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_Consent(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Post("/consent", handler.Consent)

	userID := uuid.New()
	testUser := &user.User{
		ID:            userID,
		Email:         "test@example.com",
		FirstName:     stringPtr("John"),
		LastName:      stringPtr("Doe"),
		EmailVerified: true,
	}

	tests := []struct {
		name           string
		requestBody    ConsentRequest
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid request body",
			requestBody:    ConsentRequest{},
			setupMocks:     func() {},
			expectedStatus: 400,
		},
		{
			name: "hydra client error on get consent request",
			requestBody: ConsentRequest{
				Challenge:  "test-challenge",
				GrantScope: []string{"openid", "email"},
			},
			setupMocks: func() {
				mockHydraClient.On("GetConsentRequest", "test-challenge").
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to get consent request",
		},
		{
			name: "successful consent",
			requestBody: ConsentRequest{
				Challenge:   "test-challenge",
				GrantScope:  []string{"openid", "email"},
				Remember:    true,
				RememberFor: 3600,
			},
			setupMocks: func() {
				consentReq := &hydra.ConsentRequest{
					Challenge:         "test-challenge",
					Subject:           userID.String(),
					RequestedAudience: []string{"test-audience"},
				}
				mockHydraClient.On("GetConsentRequest", "test-challenge").
					Return(consentReq, nil).Once()
				mockUserService.On("GetByID", userID).
					Return(testUser, nil).Once()
				consentResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/callback"}
				mockHydraClient.On("AcceptConsentRequest", "test-challenge", mock.AnythingOfType("*hydra.AcceptConsentRequest")).
					Return(consentResponse, nil).Once()
			},
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/consent", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_RejectConsent(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Post("/consent/reject", handler.RejectConsent)

	tests := []struct {
		name           string
		challenge      string
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing challenge parameter",
			challenge:      "",
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "consent_challenge parameter is required",
		},
		{
			name:      "hydra client error",
			challenge: "test-challenge",
			setupMocks: func() {
				mockHydraClient.On("RejectConsentRequest", "test-challenge", "access_denied", "User denied consent").
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to reject consent request",
		},
		{
			name:      "successful consent rejection",
			challenge: "test-challenge",
			setupMocks: func() {
				rejectResponse := &hydra.LoginResponse{RedirectTo: "http://example.com/error"}
				mockHydraClient.On("RejectConsentRequest", "test-challenge", "access_denied", "User denied consent").
					Return(rejectResponse, nil).Once()
			},
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			url := "/consent/reject"
			if tt.challenge != "" {
				url += "?consent_challenge=" + tt.challenge
			}

			req := httptest.NewRequest("POST", url, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_Register(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Post("/register", handler.Register)

	tests := []struct {
		name           string
		requestBody    RegisterRequest
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid request body",
			requestBody:    RegisterRequest{},
			setupMocks:     func() {},
			expectedStatus: 400,
		},
		{
			name: "missing email and password",
			requestBody: RegisterRequest{
				FirstName: "John",
				LastName:  "Doe",
			},
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "Email and password are required",
		},
		{
			name: "missing email",
			requestBody: RegisterRequest{
				Password:  "password123",
				FirstName: "John",
				LastName:  "Doe",
			},
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "Email and password are required",
		},
		{
			name: "missing password",
			requestBody: RegisterRequest{
				Email:     "test@example.com",
				FirstName: "John",
				LastName:  "Doe",
			},
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "Email and password are required",
		},
		{
			name: "user service error",
			requestBody: RegisterRequest{
				Email:     "test@example.com",
				Password:  "password123",
				FirstName: "John",
				LastName:  "Doe",
			},
			setupMocks: func() {
				createReq := &user.CreateUserRequest{
					Email:     "test@example.com",
					Password:  "password123",
					FirstName: "John",
					LastName:  "Doe",
				}
				mockUserService.On("Create", createReq).
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 500,
			expectedError:  "Failed to create user",
		},
		{
			name: "successful registration",
			requestBody: RegisterRequest{
				Email:     "test@example.com",
				Password:  "password123",
				FirstName: "John",
				LastName:  "Doe",
			},
			setupMocks: func() {
				createReq := &user.CreateUserRequest{
					Email:     "test@example.com",
					Password:  "password123",
					FirstName: "John",
					LastName:  "Doe",
				}
				createdUser := &user.User{
					ID:        uuid.New(),
					Email:     "test@example.com",
					FirstName: stringPtr("John"),
					LastName:  stringPtr("Doe"),
				}
				mockUserService.On("Create", createReq).
					Return(createdUser, nil).Once()
			},
			expectedStatus: 201,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/register", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_Profile(t *testing.T) {
	app := fiber.New()
	mockUserService := &MockUserService{}
	mockHydraClient := &MockHydraClient{}
	handler := NewAuthHandler(mockUserService, &MockClientService{}, mockHydraClient, zap.NewNop())

	app.Get("/profile/:id", handler.Profile)

	userID := uuid.New()
	testUser := &user.User{
		ID:            userID,
		Email:         "test@example.com",
		FirstName:     stringPtr("John"),
		LastName:      stringPtr("Doe"),
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	tests := []struct {
		name           string
		userIDParam    string
		setupMocks     func()
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing user ID parameter",
			userIDParam:    "",
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "User ID is required",
		},
		{
			name:           "invalid user ID format",
			userIDParam:    "invalid-uuid",
			setupMocks:     func() {},
			expectedStatus: 400,
			expectedError:  "Invalid user ID format",
		},
		{
			name:        "user not found",
			userIDParam: userID.String(),
			setupMocks: func() {
				mockUserService.On("GetByID", userID).
					Return(nil, assert.AnError).Once()
			},
			expectedStatus: 404,
			expectedError:  "User not found",
		},
		{
			name:        "successful profile retrieval",
			userIDParam: userID.String(),
			setupMocks: func() {
				mockUserService.On("GetByID", userID).
					Return(testUser, nil).Once()
			},
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockHydraClient.ExpectedCalls = nil
			mockUserService.ExpectedCalls = nil

			tt.setupMocks()

			url := "/profile/" + tt.userIDParam
			req := httptest.NewRequest("GET", url, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				assert.Contains(t, result["error"], tt.expectedError)
			}

			mockHydraClient.AssertExpectations(t)
			mockUserService.AssertExpectations(t)
		})
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
