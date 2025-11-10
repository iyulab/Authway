package client

import (
	"testing"

	"authway/apps/central/api/internal/hydra"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockHydraClient mocks the Hydra client for testing
type MockHydraClient struct {
	mock.Mock
}

func (m *MockHydraClient) CreateOAuth2Client(client *hydra.OAuth2Client) (*hydra.OAuth2Client, error) {
	args := m.Called(client)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.OAuth2Client), args.Error(1)
}

func (m *MockHydraClient) GetOAuth2Client(clientID string) (*hydra.OAuth2Client, error) {
	args := m.Called(clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.OAuth2Client), args.Error(1)
}

func (m *MockHydraClient) UpdateOAuth2Client(clientID string, client *hydra.OAuth2Client) (*hydra.OAuth2Client, error) {
	args := m.Called(clientID, client)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*hydra.OAuth2Client), args.Error(1)
}

func (m *MockHydraClient) DeleteOAuth2Client(clientID string) error {
	args := m.Called(clientID)
	return args.Error(0)
}

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Auto migrate the schema
	err = db.AutoMigrate(&Client{})
	assert.NoError(t, err)

	// Create tenants table for foreign key constraints
	err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			active BOOLEAN DEFAULT true,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)
	`).Error
	assert.NoError(t, err)

	return db
}

// createTestTenant creates a test tenant
func createTestTenant(t *testing.T, db *gorm.DB) uuid.UUID {
	tenantID := uuid.New()
	err := db.Exec(
		"INSERT INTO tenants (id, name, active, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))",
		tenantID.String(), "Test Tenant", true,
	).Error
	assert.NoError(t, err)
	return tenantID
}

// TestCreateClient_PublicWithCustomID tests public client creation with custom client_id
func TestCreateClient_PublicWithCustomID(t *testing.T) {
	db := setupTestDB(t)
	tenantID := createTestTenant(t, db)
	logger := zap.NewNop()

	// Mock Hydra client
	mockHydra := new(MockHydraClient)
	mockHydra.On("CreateOAuth2Client", mock.MatchedBy(func(c *hydra.OAuth2Client) bool {
		return c.ClientID == "my_spa_app" &&
			c.ClientSecret == "" &&
			c.TokenEndpointAuthMethod == "none"
	})).Return(&hydra.OAuth2Client{ClientID: "my_spa_app"}, nil)

	// Create service with mock
	hydraClient := &hydra.Client{}
	svc := &service{
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}

	// Override the CreateOAuth2Client method
	originalCreate := svc.hydraClient.CreateOAuth2Client
	defer func() { svc.hydraClient.CreateOAuth2Client = originalCreate }()

	// Inject mock behavior
	svc.hydraClient.CreateOAuth2Client = mockHydra.CreateOAuth2Client

	// Test data
	req := &CreateClientRequest{
		TenantID:     tenantID.String(),
		ClientID:     "my_spa_app",
		ClientSecret: "", // Empty secret for public client
		Name:         "My SPA App",
		Public:       true,
		RedirectURIs: []string{"http://localhost:5173"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
	}

	// Execute
	client, creds, err := svc.Create(req)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, creds)
	assert.Equal(t, "my_spa_app", client.ClientID)
	assert.Equal(t, "", client.ClientSecret) // Public clients have empty secret
	assert.True(t, client.Public)
	assert.Equal(t, "My SPA App", client.Name)
	assert.Equal(t, "my_spa_app", creds.ClientID)
	assert.Equal(t, "", creds.ClientSecret)

	mockHydra.AssertExpectations(t)
}

// TestCreateClient_PublicWithoutCustomID tests public client creation with auto-generated client_id
func TestCreateClient_PublicWithoutCustomID(t *testing.T) {
	db := setupTestDB(t)
	tenantID := createTestTenant(t, db)
	logger := zap.NewNop()

	// Mock Hydra client
	mockHydra := new(MockHydraClient)
	mockHydra.On("CreateOAuth2Client", mock.MatchedBy(func(c *hydra.OAuth2Client) bool {
		return c.ClientSecret == "" &&
			c.TokenEndpointAuthMethod == "none"
	})).Return(&hydra.OAuth2Client{}, nil)

	// Create service
	hydraClient := &hydra.Client{}
	svc := &service{
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}

	// Inject mock
	svc.hydraClient.CreateOAuth2Client = mockHydra.CreateOAuth2Client

	// Test data - no client_id provided
	req := &CreateClientRequest{
		TenantID:     tenantID.String(),
		Name:         "My SPA App",
		Public:       true,
		RedirectURIs: []string{"http://localhost:5173"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
	}

	// Execute
	client, creds, err := svc.Create(req)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, creds)
	assert.NotEmpty(t, client.ClientID)
	assert.Contains(t, client.ClientID, "authway_") // Auto-generated format
	assert.Equal(t, "", client.ClientSecret)
	assert.True(t, client.Public)

	mockHydra.AssertExpectations(t)
}

// TestCreateClient_ConfidentialWithBothCredentials tests confidential client with both credentials
func TestCreateClient_ConfidentialWithBothCredentials(t *testing.T) {
	db := setupTestDB(t)
	tenantID := createTestTenant(t, db)
	logger := zap.NewNop()

	// Mock Hydra client
	mockHydra := new(MockHydraClient)
	mockHydra.On("CreateOAuth2Client", mock.MatchedBy(func(c *hydra.OAuth2Client) bool {
		return c.ClientID == "my_backend" &&
			c.ClientSecret == "secure_secret_123" &&
			c.TokenEndpointAuthMethod == "client_secret_post"
	})).Return(&hydra.OAuth2Client{ClientID: "my_backend"}, nil)

	// Create service
	hydraClient := &hydra.Client{}
	svc := &service{
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}

	// Inject mock
	svc.hydraClient.CreateOAuth2Client = mockHydra.CreateOAuth2Client

	// Test data
	req := &CreateClientRequest{
		TenantID:     tenantID.String(),
		ClientID:     "my_backend",
		ClientSecret: "secure_secret_123",
		Name:         "My Backend Service",
		Public:       false, // Confidential client
		RedirectURIs: []string{"http://localhost:8080/callback"},
		GrantTypes:   []string{"authorization_code", "client_credentials"},
		Scopes:       []string{"openid", "profile", "email"},
	}

	// Execute
	client, creds, err := svc.Create(req)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, creds)
	assert.Equal(t, "my_backend", client.ClientID)
	assert.Equal(t, "secure_secret_123", client.ClientSecret)
	assert.False(t, client.Public)
	assert.Equal(t, "my_backend", creds.ClientID)
	assert.Equal(t, "secure_secret_123", creds.ClientSecret)

	mockHydra.AssertExpectations(t)
}

// TestCreateClient_ConfidentialPartialCredentials tests error when only client_id is provided
func TestCreateClient_ConfidentialPartialCredentials(t *testing.T) {
	db := setupTestDB(t)
	tenantID := createTestTenant(t, db)
	logger := zap.NewNop()
	hydraClient := &hydra.Client{}

	svc := &service{
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}

	// Test case 1: Only client_id provided
	req1 := &CreateClientRequest{
		TenantID:     tenantID.String(),
		ClientID:     "my_backend",
		ClientSecret: "", // Missing secret
		Name:         "My Backend Service",
		Public:       false,
		RedirectURIs: []string{"http://localhost:8080/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	}

	_, _, err := svc.Create(req1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must provide both client_id and client_secret")

	// Test case 2: Only client_secret provided
	req2 := &CreateClientRequest{
		TenantID:     tenantID.String(),
		ClientID:     "", // Missing client_id
		ClientSecret: "secret_123",
		Name:         "My Backend Service",
		Public:       false,
		RedirectURIs: []string{"http://localhost:8080/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	}

	_, _, err = svc.Create(req2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must provide both client_id and client_secret")
}

// TestCreateClient_ConfidentialAutoGenerate tests auto-generation for confidential clients
func TestCreateClient_ConfidentialAutoGenerate(t *testing.T) {
	db := setupTestDB(t)
	tenantID := createTestTenant(t, db)
	logger := zap.NewNop()

	// Mock Hydra client
	mockHydra := new(MockHydraClient)
	mockHydra.On("CreateOAuth2Client", mock.MatchedBy(func(c *hydra.OAuth2Client) bool {
		return c.TokenEndpointAuthMethod == "client_secret_post" &&
			c.ClientSecret != ""
	})).Return(&hydra.OAuth2Client{}, nil)

	// Create service
	hydraClient := &hydra.Client{}
	svc := &service{
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}

	// Inject mock
	svc.hydraClient.CreateOAuth2Client = mockHydra.CreateOAuth2Client

	// Test data - no credentials provided
	req := &CreateClientRequest{
		TenantID:     tenantID.String(),
		Name:         "My Backend Service",
		Public:       false, // Confidential client
		RedirectURIs: []string{"http://localhost:8080/callback"},
		GrantTypes:   []string{"authorization_code", "client_credentials"},
		Scopes:       []string{"openid", "profile"},
	}

	// Execute
	client, creds, err := svc.Create(req)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, creds)
	assert.NotEmpty(t, client.ClientID)
	assert.NotEmpty(t, client.ClientSecret)
	assert.Contains(t, client.ClientID, "authway_")
	assert.False(t, client.Public)

	mockHydra.AssertExpectations(t)
}

// TestMaskSecret tests the maskSecret helper function
func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty string", "", "(empty)"},
		{"Short secret", "abc", "****"},
		{"Normal secret", "secret123", "se****23"},
		{"Long secret", "very_long_secret_string", "ve****ng"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskSecret(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
