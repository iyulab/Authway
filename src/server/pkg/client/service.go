package client

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service interface {
	Create(req *CreateClientRequest) (*Client, *ClientCredentials, error)
	GetByID(id uuid.UUID) (*Client, error)
	GetByClientID(clientID string) (*Client, error)
	Update(id uuid.UUID, req *UpdateClientRequest) (*Client, error)
	Delete(id uuid.UUID) error
	List(limit, offset int) ([]*Client, int64, error)
	ValidateClient(clientID, clientSecret string) (*Client, error)
	RegenerateSecret(id uuid.UUID) (*ClientCredentials, error)
}

type service struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewService(db *gorm.DB, logger *zap.Logger) Service {
	return &service{
		db:     db,
		logger: logger,
	}
}

func (s *service) Create(req *CreateClientRequest) (*Client, *ClientCredentials, error) {
	// Generate client ID and secret
	clientID := s.generateClientID()
	clientSecret := s.generateClientSecret()

	client := &Client{
		ID:           uuid.New(),
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

	if err := s.db.Create(client).Error; err != nil {
		s.logger.Error("Failed to create client", zap.Error(err), zap.String("name", req.Name))
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	credentials := &ClientCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	s.logger.Info("Client created successfully",
		zap.String("id", client.ID.String()),
		zap.String("client_id", clientID),
		zap.String("name", client.Name))

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

	s.logger.Info("Client updated successfully", zap.String("id", client.ID.String()))
	return &client, nil
}

func (s *service) Delete(id uuid.UUID) error {
	result := s.db.Delete(&Client{}, id)
	if result.Error != nil {
		s.logger.Error("Failed to delete client", zap.Error(result.Error), zap.String("id", id.String()))
		return fmt.Errorf("failed to delete client: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("client not found")
	}

	s.logger.Info("Client deleted successfully", zap.String("id", id.String()))
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

	credentials := &ClientCredentials{
		ClientID:     client.ClientID,
		ClientSecret: newSecret,
	}

	s.logger.Info("Client secret regenerated successfully", zap.String("id", client.ID.String()))
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
