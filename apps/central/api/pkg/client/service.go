package client

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"authway/apps/central/api/internal/hydra"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service interface {
	Create(req *CreateClientRequest) (*Client, *ClientCredentials, error)
	GetByID(id uuid.UUID) (*Client, error)
	GetByClientID(clientID string) (*Client, error)
	GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*Client, int64, error)
	Update(id uuid.UUID, req *UpdateClientRequest) (*Client, error)
	Delete(id uuid.UUID) error
	List(limit, offset int) ([]*Client, int64, error)
	ValidateClient(clientID, clientSecret string) (*Client, error)
	RegenerateSecret(id uuid.UUID) (*ClientCredentials, error)
	SyncAllClientsToHydra() (int, int, error) // returns (synced, failed, error)
}

type service struct {
	db          *gorm.DB
	logger      *zap.Logger
	hydraClient *hydra.Client
}

func NewService(db *gorm.DB, logger *zap.Logger, hydraClient *hydra.Client) Service {
	return &service{
		db:          db,
		logger:      logger,
		hydraClient: hydraClient,
	}
}

func (s *service) Create(req *CreateClientRequest) (*Client, *ClientCredentials, error) {
	// Validate tenant_id
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid tenant_id: %w", err)
	}

	// Verify tenant exists
	var tenantExists bool
	if err := s.db.Raw("SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND active = true)", tenantID).Scan(&tenantExists).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to verify tenant: %w", err)
	}
	if !tenantExists {
		return nil, nil, fmt.Errorf("tenant not found or inactive")
	}

	// Use provided credentials or generate new ones
	// OAuth 2.0 RFC 6749 Section 2.1: Public clients cannot maintain credential confidentiality
	var clientID, clientSecret string

	if req.Public {
		// Public Client (SPA, Mobile) - client_secret not required
		if req.ClientID != "" {
			// Use provided client_id
			clientID = req.ClientID
			s.logger.Info("Using provided client_id for public client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
		} else {
			// Generate random client_id
			clientID = s.generateClientID()
			s.logger.Info("Generated client_id for public client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
		}
		clientSecret = "" // Public clients don't need secrets (use PKCE instead)

	} else {
		// Confidential Client (Backend) - both required or both generated
		if req.ClientID != "" && req.ClientSecret != "" {
			// Use provided credentials
			clientID = req.ClientID
			clientSecret = req.ClientSecret
			s.logger.Info("Using provided credentials for confidential client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))

		} else if req.ClientID != "" || req.ClientSecret != "" {
			// Partial credentials provided - error
			return nil, nil, fmt.Errorf(
				"confidential clients must provide both client_id and client_secret, or neither (got client_id='%s', client_secret='%s')",
				req.ClientID, maskSecret(req.ClientSecret))

		} else {
			// Generate both credentials
			clientID = s.generateClientID()
			clientSecret = s.generateClientSecret()
			s.logger.Info("Generated credentials for confidential client",
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
		}
	}

	// Check if client already exists with this client_id (including soft-deleted ones)
	if req.ClientID != "" {
		var existingClient Client
		err := s.db.Unscoped().Where("client_id = ?", clientID).First(&existingClient).Error
		if err == nil {
			// Client exists (either active or soft-deleted)
			if existingClient.DeletedAt.Valid {
				// Client was soft-deleted, restore it
				s.logger.Info("Client was soft-deleted, restoring it",
					zap.String("client_id", clientID),
					zap.String("tenant_id", tenantID.String()))

				// Restore the client by clearing deleted_at
				if err := s.db.Unscoped().Model(&existingClient).Update("deleted_at", nil).Error; err != nil {
					return nil, nil, fmt.Errorf("failed to restore client: %w", err)
				}

				// Update other fields if needed
				existingClient.Name = req.Name
				existingClient.Description = req.Description
				existingClient.RedirectURIs = req.RedirectURIs
				existingClient.Active = true

				// Apply smart defaults for logout redirect URIs on restore
				if len(req.PostLogoutRedirectURIs) > 0 {
					existingClient.PostLogoutRedirectURIs = req.PostLogoutRedirectURIs
				} else if len(req.RedirectURIs) > 0 {
					existingClient.PostLogoutRedirectURIs = req.RedirectURIs
				}
				if req.LogoutRedirectPolicy != "" {
					existingClient.LogoutRedirectPolicy = req.LogoutRedirectPolicy
				} else if existingClient.LogoutRedirectPolicy == "" {
					existingClient.LogoutRedirectPolicy = "strict"
				}
				if req.DefaultLogoutURI != "" {
					existingClient.DefaultLogoutURI = &req.DefaultLogoutURI
				} else if existingClient.DefaultLogoutURI == nil && len(req.RedirectURIs) > 0 {
					defaultURI := req.RedirectURIs[0]
					existingClient.DefaultLogoutURI = &defaultURI
				}

				if err := s.db.Save(&existingClient).Error; err != nil {
					return nil, nil, fmt.Errorf("failed to update restored client: %w", err)
				}
			} else {
				// Client is active, just return it
				s.logger.Info("Client already exists, returning existing client",
					zap.String("client_id", clientID),
					zap.String("tenant_id", tenantID.String()))
			}

			credentials := &ClientCredentials{
				ClientID:     existingClient.ClientID,
				ClientSecret: clientSecret, // Return the secret (empty for public clients)
			}

			return &existingClient, credentials, nil
		} else if err != gorm.ErrRecordNotFound {
			// Real database error
			return nil, nil, fmt.Errorf("failed to check existing client: %w", err)
		}
		// Client doesn't exist, proceed with creation
	}

	client := &Client{
		ID:           uuid.New(),
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Name:         req.Name,
		Description:  req.Description,
		Website:      req.Website,
		Logo:         req.Logo,
		RedirectURIs: req.RedirectURIs,
		GrantTypes:   req.GrantTypes,
		Scopes:       req.Scopes,
		Public:       req.Public,
		Active:       true,
	}

	// Set Google OAuth if provided
	if req.GoogleOAuthEnabled {
		client.GoogleOAuthEnabled = true
		if req.GoogleClientID != "" {
			client.GoogleClientID = &req.GoogleClientID
		}
		if req.GoogleClientSecret != "" {
			client.GoogleClientSecret = &req.GoogleClientSecret
		}
		if req.GoogleRedirectURI != "" {
			client.GoogleRedirectURI = &req.GoogleRedirectURI
		}
	}

	// Set GitHub OAuth if provided
	if req.GithubOAuthEnabled {
		client.GithubOAuthEnabled = true
		if req.GithubClientID != "" {
			client.GithubClientID = &req.GithubClientID
		}
		if req.GithubClientSecret != "" {
			client.GithubClientSecret = &req.GithubClientSecret
		}
	}

	// Set allowed origins for CORS validation
	if len(req.AllowedOrigins) > 0 {
		client.AllowedOrigins = req.AllowedOrigins
	}

	// Logout Redirect Policy Configuration (Smart Defaults)
	// Philosophy: Minimize boilerplate - auto-populate from redirect_uris if not explicitly set
	if len(req.PostLogoutRedirectURIs) > 0 {
		// Use explicitly provided URIs
		client.PostLogoutRedirectURIs = req.PostLogoutRedirectURIs
	} else if len(req.RedirectURIs) > 0 {
		// Smart default: use redirect_uris as post_logout_redirect_uris
		// This follows the common pattern used by Auth0, Okta, and other providers
		client.PostLogoutRedirectURIs = req.RedirectURIs
		s.logger.Debug("Auto-populated post_logout_redirect_uris from redirect_uris",
			zap.String("client_id", clientID),
			zap.Strings("uris", req.RedirectURIs))
	}

	// Set logout redirect policy (default: "strict" for production safety)
	if req.LogoutRedirectPolicy != "" {
		client.LogoutRedirectPolicy = req.LogoutRedirectPolicy
	} else {
		client.LogoutRedirectPolicy = "strict"
	}

	// Set default logout URI - use first redirect URI if not provided
	if req.DefaultLogoutURI != "" {
		client.DefaultLogoutURI = &req.DefaultLogoutURI
	} else if len(req.RedirectURIs) > 0 {
		defaultURI := req.RedirectURIs[0]
		client.DefaultLogoutURI = &defaultURI
	}

	// Allow wildcard logout (default: false for security)
	client.AllowWildcardLogout = req.AllowWildcardLogout

	if err := s.db.Create(client).Error; err != nil {
		s.logger.Error("Failed to create client", zap.Error(err), zap.String("name", req.Name), zap.String("tenant_id", tenantID.String()))
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Register client in Hydra
	// Set token_endpoint_auth_method based on public client setting
	authMethod := "client_secret_post"
	if client.Public {
		authMethod = "none"
	}

	// Smart fallback: Ensure Hydra always has valid post_logout_redirect_uris
	hydraPostLogoutURIs := client.PostLogoutRedirectURIs
	if len(hydraPostLogoutURIs) == 0 && len(client.RedirectURIs) > 0 {
		hydraPostLogoutURIs = client.RedirectURIs
	}

	hydraClient := &hydra.OAuth2Client{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              client.Name,
		RedirectUris:            client.RedirectURIs,
		PostLogoutRedirectUris:  hydraPostLogoutURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           []string{"code"}, // Default to authorization code flow
		Scope:                   strings.Join(client.Scopes, " "),
		TokenEndpointAuthMethod: authMethod,
	}

	// DEBUG: Log Hydra Client AdminURL before making request
	s.logger.Info("🔍 DEBUG: About to call Hydra CreateOAuth2Client",
		zap.String("hydra_admin_url", s.hydraClient.AdminURL),
		zap.String("client_id", clientID))

	_, err = s.hydraClient.CreateOAuth2Client(hydraClient)
	if err != nil {
		// Check if error is due to client already existing in Hydra
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "Conflict") {
			s.logger.Info("Client already exists in Hydra, skipping registration",
				zap.String("client_id", clientID))
			// Don't fail, client exists in both systems now
		} else {
			// Real Hydra error - rollback database creation
			s.db.Delete(client)
			s.logger.Error("Failed to register client in Hydra, rolled back database",
				zap.Error(err),
				zap.String("client_id", clientID),
				zap.String("tenant_id", tenantID.String()))
			return nil, nil, fmt.Errorf("failed to register client in Hydra: %w", err)
		}
	}

	credentials := &ClientCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	s.logger.Info("Client created successfully in database and Hydra",
		zap.String("id", client.ID.String()),
		zap.String("client_id", clientID),
		zap.String("name", client.Name),
		zap.String("tenant_id", tenantID.String()))

	return client, credentials, nil
}

func (s *service) GetByID(id uuid.UUID) (*Client, error) {
	var client Client
	if err := s.db.Where("id = ?", id).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return &client, nil
}

func (s *service) GetByClientID(clientID string) (*Client, error) {
	var client Client
	if err := s.db.Where("client_id = ?", clientID).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return &client, nil
}

func (s *service) Update(id uuid.UUID, req *UpdateClientRequest) (*Client, error) {
	var client Client
	if err := s.db.Where("id = ?", id).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	// Update fields
	// Note: For string fields, we update even if empty to allow clearing values
	// For array fields, nil means "not provided" while empty slice means "clear the values"
	if req.Name != "" {
		client.Name = req.Name
	}
	// Allow clearing description (empty string is valid)
	client.Description = req.Description
	// Allow clearing website (empty string is valid)
	client.Website = req.Website
	if req.Logo != "" {
		client.Logo = req.Logo
	}

	// Array fields: update if slice is not nil (allows clearing with empty array)
	if req.RedirectURIs != nil {
		client.RedirectURIs = req.RedirectURIs
	}
	if req.GrantTypes != nil {
		client.GrantTypes = req.GrantTypes
	}
	if req.Scopes != nil {
		client.Scopes = req.Scopes
	}
	if req.Public != nil {
		client.Public = *req.Public
	}
	if req.Active != nil {
		client.Active = *req.Active
	}

	// Google OAuth settings
	if req.GoogleOAuthEnabled != nil {
		client.GoogleOAuthEnabled = *req.GoogleOAuthEnabled
	}
	if req.GoogleClientID != nil {
		client.GoogleClientID = req.GoogleClientID
	}
	if req.GoogleClientSecret != nil {
		client.GoogleClientSecret = req.GoogleClientSecret
	}
	if req.GoogleRedirectURI != nil {
		client.GoogleRedirectURI = req.GoogleRedirectURI
	}

	// Update allowed origins if provided (nil = not provided, empty = clear)
	if req.AllowedOrigins != nil {
		client.AllowedOrigins = req.AllowedOrigins
	}

	// Logout redirect policy settings
	// PostLogoutRedirectURIs: nil = not provided, empty = clear values
	if req.PostLogoutRedirectURIs != nil {
		client.PostLogoutRedirectURIs = req.PostLogoutRedirectURIs
	}
	if req.LogoutRedirectPolicy != nil {
		client.LogoutRedirectPolicy = *req.LogoutRedirectPolicy
	}
	// DefaultLogoutURI: nil = not provided, empty string = clear value (set to nil)
	if req.DefaultLogoutURI != nil {
		if *req.DefaultLogoutURI == "" {
			client.DefaultLogoutURI = nil // Clear the value
		} else {
			client.DefaultLogoutURI = req.DefaultLogoutURI
		}
	}
	if req.AllowWildcardLogout != nil {
		client.AllowWildcardLogout = *req.AllowWildcardLogout
	}

	if err := s.db.Save(&client).Error; err != nil {
		s.logger.Error("Failed to update client", zap.Error(err), zap.String("id", id.String()))
		return nil, fmt.Errorf("failed to update client: %w", err)
	}

	// Update client in Hydra
	// Set token_endpoint_auth_method based on public client setting
	authMethod := "client_secret_post"
	if client.Public {
		authMethod = "none"
	}

	// Smart fallback: If PostLogoutRedirectURIs is empty, use RedirectURIs for Hydra
	// This ensures logout always works even if user clears the explicit list
	hydraPostLogoutURIs := client.PostLogoutRedirectURIs
	if len(hydraPostLogoutURIs) == 0 && len(client.RedirectURIs) > 0 {
		hydraPostLogoutURIs = client.RedirectURIs
		s.logger.Debug("Using redirect_uris as fallback for empty post_logout_redirect_uris in Hydra",
			zap.String("client_id", client.ClientID))
	}

	hydraUpdate := &hydra.OAuth2Client{
		ClientID:                client.ClientID,
		ClientSecret:            client.ClientSecret,
		ClientName:              client.Name,
		RedirectUris:            client.RedirectURIs,
		PostLogoutRedirectUris:  hydraPostLogoutURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           []string{"code"},
		Scope:                   strings.Join(client.Scopes, " "),
		TokenEndpointAuthMethod: authMethod,
	}

	_, errHydra := s.hydraClient.UpdateOAuth2Client(client.ClientID, hydraUpdate)
	if errHydra != nil {
		s.logger.Warn("Failed to update client in Hydra (database updated)",
			zap.Error(errHydra),
			zap.String("client_id", client.ClientID))
		// Don't rollback database - Hydra update is best-effort
	}

	s.logger.Info("Client updated successfully in database and Hydra", zap.String("id", client.ID.String()))
	return &client, nil
}

func (s *service) Delete(id uuid.UUID) error {
	// Get client first to retrieve client_id for Hydra deletion
	client, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// Delete from Hydra first
	err = s.hydraClient.DeleteOAuth2Client(client.ClientID)
	if err != nil {
		s.logger.Warn("Failed to delete client from Hydra (proceeding with database deletion)",
			zap.Error(err),
			zap.String("client_id", client.ClientID))
		// Continue with database deletion even if Hydra deletion fails
	}

	// Delete from database
	result := s.db.Delete(&Client{}, id)
	if result.Error != nil {
		s.logger.Error("Failed to delete client", zap.Error(result.Error), zap.String("id", id.String()))
		return fmt.Errorf("failed to delete client: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("client not found")
	}

	s.logger.Info("Client deleted successfully from database and Hydra", zap.String("id", id.String()))
	return nil
}

func (s *service) List(limit, offset int) ([]*Client, int64, error) {
	var clients []*Client
	var total int64

	// Get total count
	if err := s.db.Model(&Client{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count clients: %w", err)
	}

	// Get clients with pagination
	if err := s.db.Limit(limit).Offset(offset).Find(&clients).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list clients: %w", err)
	}

	return clients, total, nil
}

func (s *service) ValidateClient(clientID, clientSecret string) (*Client, error) {
	client, err := s.GetByClientID(clientID)
	if err != nil {
		return nil, err
	}

	if !client.Active {
		return nil, fmt.Errorf("client is not active")
	}

	// For public clients, don't validate secret
	if client.Public {
		return client, nil
	}

	// Validate client secret for confidential clients
	if client.ClientSecret != clientSecret {
		return nil, fmt.Errorf("invalid client credentials")
	}

	return client, nil
}

func (s *service) RegenerateSecret(id uuid.UUID) (*ClientCredentials, error) {
	client, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Generate new secret
	newSecret := s.generateClientSecret()

	if err := s.db.Model(client).Update("client_secret", newSecret).Error; err != nil {
		s.logger.Error("Failed to regenerate client secret", zap.Error(err), zap.String("id", id.String()))
		return nil, fmt.Errorf("failed to regenerate client secret: %w", err)
	}

	// Update secret in Hydra
	// Set token_endpoint_auth_method based on public client setting
	authMethod := "client_secret_post"
	if client.Public {
		authMethod = "none"
	}

	hydraUpdate := &hydra.OAuth2Client{
		ClientID:                client.ClientID,
		ClientSecret:            newSecret,
		ClientName:              client.Name,
		RedirectUris:            client.RedirectURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           []string{"code"},
		Scope:                   strings.Join(client.Scopes, " "),
		TokenEndpointAuthMethod: authMethod,
	}

	_, errHydra := s.hydraClient.UpdateOAuth2Client(client.ClientID, hydraUpdate)
	if errHydra != nil {
		s.logger.Warn("Failed to update client secret in Hydra (database updated)",
			zap.Error(errHydra),
			zap.String("client_id", client.ClientID))
		// Don't rollback - database update is primary
	}

	credentials := &ClientCredentials{
		ClientID:     client.ClientID,
		ClientSecret: newSecret,
	}

	s.logger.Info("Client secret regenerated successfully in database and Hydra", zap.String("id", client.ID.String()))
	return credentials, nil
}

func (s *service) generateClientID() string {
	// Generate a random client ID
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("authway_%s", base64.URLEncoding.EncodeToString(bytes)[:22])
}

func (s *service) generateClientSecret() string {
	// Generate a random client secret
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return strings.ReplaceAll(base64.URLEncoding.EncodeToString(bytes), "=", "")
}

// GetByTenant retrieves all clients for a specific tenant with pagination
func (s *service) GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*Client, int64, error) {
	var clients []*Client
	var total int64

	// Get total count for this tenant
	if err := s.db.Model(&Client{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count clients: %w", err)
	}

	// Get clients with pagination
	if err := s.db.Where("tenant_id = ?", tenantID).Limit(limit).Offset(offset).Find(&clients).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list clients: %w", err)
	}

	return clients, total, nil
}

// maskSecret masks a secret string for logging purposes
func maskSecret(secret string) string {
	if secret == "" {
		return "(empty)"
	}
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}

// SyncAllClientsToHydra synchronizes all clients' post_logout_redirect_uris to Hydra
// This is useful for one-time migration of existing clients that have empty post_logout_redirect_uris in Hydra
// For each client, if post_logout_redirect_uris is empty, it uses redirect_uris as fallback
func (s *service) SyncAllClientsToHydra() (int, int, error) {
	s.logger.Info("Starting Hydra sync for all clients")

	var clients []*Client
	if err := s.db.Find(&clients).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to fetch clients: %w", err)
	}

	synced := 0
	failed := 0

	for _, client := range clients {
		// Determine post_logout_redirect_uris to use
		hydraPostLogoutURIs := client.PostLogoutRedirectURIs
		if len(hydraPostLogoutURIs) == 0 && len(client.RedirectURIs) > 0 {
			hydraPostLogoutURIs = client.RedirectURIs
			s.logger.Debug("Using redirect_uris as fallback for empty post_logout_redirect_uris",
				zap.String("client_id", client.ClientID))
		}

		// Skip if still no URIs to set
		if len(hydraPostLogoutURIs) == 0 {
			s.logger.Debug("Skipping client with no redirect URIs",
				zap.String("client_id", client.ClientID))
			continue
		}

		// Update Hydra with the URIs
		err := s.hydraClient.UpdateClient(client.ClientID, map[string]interface{}{
			"post_logout_redirect_uris": hydraPostLogoutURIs,
		})

		if err != nil {
			s.logger.Error("Failed to sync client to Hydra",
				zap.String("client_id", client.ClientID),
				zap.Error(err))
			failed++
			continue
		}

		s.logger.Debug("Successfully synced client to Hydra",
			zap.String("client_id", client.ClientID),
			zap.Strings("post_logout_redirect_uris", hydraPostLogoutURIs))
		synced++
	}

	s.logger.Info("Hydra sync completed",
		zap.Int("synced", synced),
		zap.Int("failed", failed),
		zap.Int("total", len(clients)))

	return synced, failed, nil
}
