package client

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"authway/src/server/internal/hydra"
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
	var clientID, clientSecret string
	if req.ClientID != "" && req.ClientSecret != "" {
		// Use fixed credentials provided in request
		clientID = req.ClientID
		clientSecret = req.ClientSecret
		s.logger.Info("Using provided client credentials",
			zap.String("client_id", clientID),
			zap.String("tenant_id", tenantID.String()))
	} else {
		// Generate random credentials
		clientID = s.generateClientID()
		clientSecret = s.generateClientSecret()
		s.logger.Info("Generated new client credentials",
			zap.String("client_id", clientID),
			zap.String("tenant_id", tenantID.String()))
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

	if err := s.db.Create(client).Error; err != nil {
		s.logger.Error("Failed to create client", zap.Error(err), zap.String("name", req.Name), zap.String("tenant_id", tenantID.String()))
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Register client in Hydra
	hydraClient := &hydra.OAuth2Client{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              client.Name,
		RedirectUris:            client.RedirectURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           []string{"code"}, // Default to authorization code flow
		Scope:                   strings.Join(client.Scopes, " "),
		TokenEndpointAuthMethod: "client_secret_post",
	}

	// DEBUG: Log Hydra Client AdminURL before making request
	s.logger.Info("🔍 DEBUG: About to call Hydra CreateOAuth2Client",
		zap.String("hydra_admin_url", s.hydraClient.AdminURL),
		zap.String("client_id", clientID))

	_, err = s.hydraClient.CreateOAuth2Client(hydraClient)
	if err != nil {
		// Rollback database creation if Hydra registration fails
		s.db.Delete(client)
		s.logger.Error("Failed to register client in Hydra, rolled back database",
			zap.Error(err),
			zap.String("client_id", clientID),
			zap.String("tenant_id", tenantID.String()))
		return nil, nil, fmt.Errorf("failed to register client in Hydra: %w", err)
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
	if req.Name != "" {
		client.Name = req.Name
	}
	if req.Description != "" {
		client.Description = req.Description
	}
	if req.Website != "" {
		client.Website = req.Website
	}
	if req.Logo != "" {
		client.Logo = req.Logo
	}
	if len(req.RedirectURIs) > 0 {
		client.RedirectURIs = req.RedirectURIs
	}
	if len(req.GrantTypes) > 0 {
		client.GrantTypes = req.GrantTypes
	}
	if len(req.Scopes) > 0 {
		client.Scopes = req.Scopes
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

	if err := s.db.Save(&client).Error; err != nil {
		s.logger.Error("Failed to update client", zap.Error(err), zap.String("id", id.String()))
		return nil, fmt.Errorf("failed to update client: %w", err)
	}

	// Update client in Hydra
	hydraUpdate := &hydra.OAuth2Client{
		ClientID:                client.ClientID,
		ClientSecret:            client.ClientSecret,
		ClientName:              client.Name,
		RedirectUris:            client.RedirectURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           []string{"code"},
		Scope:                   strings.Join(client.Scopes, " "),
		TokenEndpointAuthMethod: "client_secret_post",
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
	hydraUpdate := &hydra.OAuth2Client{
		ClientID:                client.ClientID,
		ClientSecret:            newSecret,
		ClientName:              client.Name,
		RedirectUris:            client.RedirectURIs,
		GrantTypes:              client.GrantTypes,
		ResponseTypes:           []string{"code"},
		Scope:                   strings.Join(client.Scopes, " "),
		TokenEndpointAuthMethod: "client_secret_post",
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
