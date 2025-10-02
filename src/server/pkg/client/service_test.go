package client

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate the schema
	err = db.AutoMigrate(&Client{})
	require.NoError(t, err)

	return db
}

func TestNewService(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)

	service := NewService(db, logger)

	assert.NotNil(t, service)
	assert.IsType(t, &service{}, service)
}

func TestService_Create(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	tests := []struct {
		name        string
		request     *CreateClientRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful client creation",
			request: &CreateClientRequest{
				Name:         "Test App",
				Description:  "Test application",
				Website:      "https://example.com",
				Logo:         "https://example.com/logo.png",
				RedirectURIs: []string{"https://example.com/callback"},
				GrantTypes:   []string{"authorization_code", "refresh_token"},
				Scopes:       []string{"openid", "email", "profile"},
				Public:       false,
			},
			expectError: false,
		},
		{
			name: "successful public client creation",
			request: &CreateClientRequest{
				Name:         "Public App",
				Description:  "Public application",
				RedirectURIs: []string{"https://example.com/callback"},
				GrantTypes:   []string{"authorization_code"},
				Scopes:       []string{"openid"},
				Public:       true,
			},
			expectError: false,
		},
		{
			name: "client with Google OAuth settings",
			request: &CreateClientRequest{
				Name:               "Google OAuth App",
				Description:        "App with Google OAuth",
				RedirectURIs:       []string{"https://example.com/callback"},
				GrantTypes:         []string{"authorization_code"},
				Scopes:             []string{"openid", "email"},
				GoogleOAuthEnabled: true,
				GoogleClientID:     "google-client-id",
				GoogleClientSecret: "google-client-secret",
				GoogleRedirectURI:  "https://example.com/auth/google/callback",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, credentials, err := service.Create(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, client)
				assert.Nil(t, credentials)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, credentials)

				// Verify client properties
				assert.NotEmpty(t, client.ID)
				assert.NotEmpty(t, client.ClientID)
				assert.NotEmpty(t, client.ClientSecret)
				assert.Equal(t, tt.request.Name, client.Name)
				assert.Equal(t, tt.request.Description, client.Description)
				assert.Equal(t, tt.request.Website, client.Website)
				assert.Equal(t, tt.request.Logo, client.Logo)
				assert.Equal(t, pq.StringArray(tt.request.RedirectURIs), client.RedirectURIs)
				assert.Equal(t, pq.StringArray(tt.request.GrantTypes), client.GrantTypes)
				assert.Equal(t, pq.StringArray(tt.request.Scopes), client.Scopes)
				assert.Equal(t, tt.request.Public, client.Public)
				assert.True(t, client.Active)

				// Verify credentials
				assert.Equal(t, client.ClientID, credentials.ClientID)
				assert.Equal(t, client.ClientSecret, credentials.ClientSecret)

				// Verify Google OAuth settings
				assert.Equal(t, tt.request.GoogleOAuthEnabled, client.GoogleOAuthEnabled)
				if tt.request.GoogleOAuthEnabled {
					assert.Equal(t, tt.request.GoogleClientID, *client.GoogleClientID)
					assert.Equal(t, tt.request.GoogleClientSecret, *client.GoogleClientSecret)
					assert.Equal(t, tt.request.GoogleRedirectURI, *client.GoogleRedirectURI)
				}

				// Verify client ID generation format
				assert.Contains(t, credentials.ClientID, "authway_")
				assert.True(t, len(credentials.ClientID) > 10)

				// Verify client secret generation
				assert.True(t, len(credentials.ClientSecret) > 40)
			}
		})
	}
}

func TestService_GetByID(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test client
	testClient, _, err := service.Create(&CreateClientRequest{
		Name:         "Test App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		clientID    uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by ID",
			clientID:    testClient.ID,
			expectError: false,
		},
		{
			name:        "client not found",
			clientID:    uuid.New(),
			expectError: true,
			errorMsg:    "client not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := service.GetByID(tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.clientID, client.ID)
			}
		})
	}
}

func TestService_GetByClientID(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test client
	testClient, _, err := service.Create(&CreateClientRequest{
		Name:         "Test App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		clientID    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful get by client ID",
			clientID:    testClient.ClientID,
			expectError: false,
		},
		{
			name:        "client not found",
			clientID:    "nonexistent-client-id",
			expectError: true,
			errorMsg:    "client not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := service.GetByClientID(tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.clientID, client.ClientID)
				assert.Equal(t, testClient.ID, client.ID)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test client
	testClient, _, err := service.Create(&CreateClientRequest{
		Name:         "Original App",
		Description:  "Original description",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		clientID    uuid.UUID
		request     *UpdateClientRequest
		expectError bool
		errorMsg    string
	}{
		{
			name:     "successful update",
			clientID: testClient.ID,
			request: &UpdateClientRequest{
				Name:         "Updated App",
				Description:  "Updated description",
				Website:      "https://updated.example.com",
				RedirectURIs: []string{"https://updated.example.com/callback"},
				GrantTypes:   []string{"authorization_code", "refresh_token"},
				Scopes:       []string{"openid", "email", "profile"},
			},
			expectError: false,
		},
		{
			name:     "partial update",
			clientID: testClient.ID,
			request: &UpdateClientRequest{
				Name: "Partially Updated App",
			},
			expectError: false,
		},
		{
			name:     "update with Google OAuth settings",
			clientID: testClient.ID,
			request: &UpdateClientRequest{
				GoogleOAuthEnabled: boolPtr(true),
				GoogleClientID:     stringPtr("new-google-client-id"),
				GoogleClientSecret: stringPtr("new-google-client-secret"),
				GoogleRedirectURI:  stringPtr("https://example.com/auth/google"),
			},
			expectError: false,
		},
		{
			name:     "update active status",
			clientID: testClient.ID,
			request: &UpdateClientRequest{
				Active: boolPtr(false),
			},
			expectError: false,
		},
		{
			name:        "client not found",
			clientID:    uuid.New(),
			request:     &UpdateClientRequest{Name: "Not Found"},
			expectError: true,
			errorMsg:    "client not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := service.Update(tt.clientID, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.clientID, client.ID)

				// Verify updates
				if tt.request.Name != "" {
					assert.Equal(t, tt.request.Name, client.Name)
				}
				if tt.request.Description != "" {
					assert.Equal(t, tt.request.Description, client.Description)
				}
				if tt.request.Website != "" {
					assert.Equal(t, tt.request.Website, client.Website)
				}
				if len(tt.request.RedirectURIs) > 0 {
					assert.Equal(t, pq.StringArray(tt.request.RedirectURIs), client.RedirectURIs)
				}
				if len(tt.request.GrantTypes) > 0 {
					assert.Equal(t, pq.StringArray(tt.request.GrantTypes), client.GrantTypes)
				}
				if len(tt.request.Scopes) > 0 {
					assert.Equal(t, pq.StringArray(tt.request.Scopes), client.Scopes)
				}
				if tt.request.Active != nil {
					assert.Equal(t, *tt.request.Active, client.Active)
				}
				if tt.request.GoogleOAuthEnabled != nil {
					assert.Equal(t, *tt.request.GoogleOAuthEnabled, client.GoogleOAuthEnabled)
				}
				if tt.request.GoogleClientID != nil {
					assert.Equal(t, *tt.request.GoogleClientID, *client.GoogleClientID)
				}
				if tt.request.GoogleClientSecret != nil {
					assert.Equal(t, *tt.request.GoogleClientSecret, *client.GoogleClientSecret)
				}
				if tt.request.GoogleRedirectURI != nil {
					assert.Equal(t, *tt.request.GoogleRedirectURI, *client.GoogleRedirectURI)
				}
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test client
	testClient, _, err := service.Create(&CreateClientRequest{
		Name:         "Test App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		clientID    uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful delete",
			clientID:    testClient.ID,
			expectError: false,
		},
		{
			name:        "client not found",
			clientID:    uuid.New(),
			expectError: true,
			errorMsg:    "client not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.Delete(tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)

				// Verify client is deleted
				_, getErr := service.GetByID(tt.clientID)
				assert.Error(t, getErr)
				assert.Contains(t, getErr.Error(), "client not found")
			}
		})
	}
}

func TestService_List(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create multiple test clients
	clients := make([]*Client, 5)
	for i := 0; i < 5; i++ {
		client, _, err := service.Create(&CreateClientRequest{
			Name:         fmt.Sprintf("Client %d", i),
			RedirectURIs: []string{fmt.Sprintf("https://example%d.com/callback", i)},
			GrantTypes:   []string{"authorization_code"},
			Scopes:       []string{"openid"},
		})
		require.NoError(t, err)
		clients[i] = client
	}

	tests := []struct {
		name          string
		limit         int
		offset        int
		expectedLen   int
		expectedTotal int64
	}{
		{
			name:          "get all clients",
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
			clients, total, err := service.List(tt.limit, tt.offset)

			assert.NoError(t, err)
			assert.Len(t, clients, tt.expectedLen)
			assert.Equal(t, tt.expectedTotal, total)

			// Verify clients are not nil
			for _, client := range clients {
				assert.NotNil(t, client)
				assert.NotEmpty(t, client.ID)
				assert.NotEmpty(t, client.ClientID)
				assert.NotEmpty(t, client.Name)
			}
		})
	}
}

func TestService_ValidateClient(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create confidential client
	confidentialClient, credentials, err := service.Create(&CreateClientRequest{
		Name:         "Confidential App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		Public:       false,
	})
	require.NoError(t, err)

	// Create public client
	publicClient, publicCredentials, err := service.Create(&CreateClientRequest{
		Name:         "Public App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		Public:       true,
	})
	require.NoError(t, err)

	// Create inactive client
	inactiveClient, inactiveCredentials, err := service.Create(&CreateClientRequest{
		Name:         "Inactive App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		Public:       false,
	})
	require.NoError(t, err)

	// Make client inactive
	_, err = service.Update(inactiveClient.ID, &UpdateClientRequest{
		Active: boolPtr(false),
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "successful confidential client validation",
			clientID:     credentials.ClientID,
			clientSecret: credentials.ClientSecret,
			expectError:  false,
		},
		{
			name:         "successful public client validation",
			clientID:     publicCredentials.ClientID,
			clientSecret: "", // Public clients don't need secret
			expectError:  false,
		},
		{
			name:         "client not found",
			clientID:     "nonexistent-client",
			clientSecret: "secret",
			expectError:  true,
			errorMsg:     "client not found",
		},
		{
			name:         "inactive client",
			clientID:     inactiveCredentials.ClientID,
			clientSecret: inactiveCredentials.ClientSecret,
			expectError:  true,
			errorMsg:     "client is not active",
		},
		{
			name:         "invalid client secret",
			clientID:     credentials.ClientID,
			clientSecret: "wrong-secret",
			expectError:  true,
			errorMsg:     "invalid client credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := service.ValidateClient(tt.clientID, tt.clientSecret)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.clientID, client.ClientID)
				assert.True(t, client.Active)
			}
		})
	}
}

func TestService_RegenerateSecret(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	service := NewService(db, logger)

	// Create test client
	testClient, originalCredentials, err := service.Create(&CreateClientRequest{
		Name:         "Test App",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		clientID    uuid.UUID
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful secret regeneration",
			clientID:    testClient.ID,
			expectError: false,
		},
		{
			name:        "client not found",
			clientID:    uuid.New(),
			expectError: true,
			errorMsg:    "client not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials, err := service.RegenerateSecret(tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, credentials)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, credentials)
				assert.Equal(t, testClient.ClientID, credentials.ClientID)
				assert.NotEqual(t, originalCredentials.ClientSecret, credentials.ClientSecret)
				assert.NotEmpty(t, credentials.ClientSecret)
				assert.True(t, len(credentials.ClientSecret) > 40)

				// Verify the secret was updated in the database
				updatedClient, getErr := service.GetByID(tt.clientID)
				require.NoError(t, getErr)
				assert.Equal(t, credentials.ClientSecret, updatedClient.ClientSecret)
			}
		})
	}
}

func TestService_GenerateClientID(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	svc := &service{db: db, logger: logger}

	clientID := svc.generateClientID()

	assert.NotEmpty(t, clientID)
	assert.Contains(t, clientID, "authway_")
	assert.True(t, len(clientID) > 10)
}

func TestService_GenerateClientSecret(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	svc := &service{db: db, logger: logger}

	clientSecret := svc.generateClientSecret()

	assert.NotEmpty(t, clientSecret)
	assert.True(t, len(clientSecret) > 40)
	assert.NotContains(t, clientSecret, "=") // Base64 padding should be removed
}

func TestClient_BeforeCreate(t *testing.T) {
	client := &Client{}

	err := client.BeforeCreate(nil)

	assert.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, client.ID)
}

func TestClient_ToPublic(t *testing.T) {
	client := &Client{
		ID:           uuid.New(),
		ClientID:     "test-client-id",
		ClientSecret: "secret-should-not-be-included",
		Name:         "Test App",
		Description:  "Test application",
		Website:      "https://example.com",
		Logo:         "https://example.com/logo.png",
		RedirectURIs: pq.StringArray{"https://example.com/callback"},
		GrantTypes:   pq.StringArray{"authorization_code"},
		Scopes:       pq.StringArray{"openid", "email"},
		Public:       false,
		Active:       true,

		GoogleOAuthEnabled: true,
		GoogleClientID:     stringPtr("google-client-id"),
		GoogleClientSecret: stringPtr("google-secret-should-not-be-included"),
		GoogleRedirectURI:  stringPtr("https://example.com/auth/google"),
	}

	publicClient := client.ToPublic()

	// Verify public fields are included
	assert.Equal(t, client.ID, publicClient.ID)
	assert.Equal(t, client.ClientID, publicClient.ClientID)
	assert.Equal(t, client.Name, publicClient.Name)
	assert.Equal(t, client.Description, publicClient.Description)
	assert.Equal(t, client.Website, publicClient.Website)
	assert.Equal(t, client.Logo, publicClient.Logo)
	assert.Equal(t, []string(client.RedirectURIs), publicClient.RedirectURIs)
	assert.Equal(t, []string(client.GrantTypes), publicClient.GrantTypes)
	assert.Equal(t, []string(client.Scopes), publicClient.Scopes)
	assert.Equal(t, client.Public, publicClient.Public)
	assert.Equal(t, client.Active, publicClient.Active)

	// Verify Google OAuth public fields are included
	assert.Equal(t, client.GoogleOAuthEnabled, publicClient.GoogleOAuthEnabled)
	assert.Equal(t, client.GoogleRedirectURI, publicClient.GoogleRedirectURI)

	// Verify secrets are not included in public client (verified by struct definition)
	// PublicClient struct doesn't have ClientSecret, GoogleClientID, or GoogleClientSecret fields
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
