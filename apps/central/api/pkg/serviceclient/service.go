package serviceclient

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
)

// allowedScopes is the initial admin-API scope allowlist a service_client
// may be granted. Extend this (and the CreateServiceClientRequest.Scopes
// validator tag) when a second admin scope (e.g. admin.invitations:write)
// is actually needed by a consumer; do not pre-add scopes nothing requests
// yet.
var allowedScopes = map[string]bool{
	"admin.clients:write": true,
}

// The grant_types a scoped-service-authenticated client-creation request may
// request are restricted separately, at the request-validation layer for
// that endpoint — a service_client must never be able to mint another
// client_credentials client. That restriction has nothing to do with this
// package, which always registers grant_types=[client_credentials] for the
// service_client's own Hydra registration, unconditionally, so no
// grant-type allowlist lives here.

type CreateServiceClientRequest struct {
	Name   string   `json:"name" validate:"required"`
	Scopes []string `json:"scopes" validate:"required,min=1,dive,oneof=admin.clients:write"`
}

type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type Service interface {
	// Create provisions a new service_client: generates credentials,
	// registers a client_credentials OAuth2Client in Hydra, and persists the
	// tenant + scope mapping. ClientCredentials.ClientSecret is returned
	// exactly once — it is never stored by this service.
	Create(tenantID uuid.UUID, req *CreateServiceClientRequest) (*ServiceClient, *ClientCredentials, error)
	// GetByHydraClientID looks up the tenant/scope mapping for a Hydra
	// client_id extracted from a validated introspection response. Used by
	// the scoped-service-auth middleware path.
	GetByHydraClientID(hydraClientID string) (*ServiceClient, error)
	// ListByTenant returns a tenant's service_client credentials, paginated,
	// newest first. Never carries a secret — ServiceClient itself has no
	// secret field; the raw secret exists only transiently in the
	// ClientCredentials Create returns once.
	ListByTenant(tenantID uuid.UUID, limit, offset int) ([]*ServiceClient, int64, error)
	// Revoke marks the credential revoked (blocking every future request
	// through GetByHydraClientID's caller regardless of Hydra state) and
	// best-effort deletes the underlying Hydra client so no new token can be
	// minted against it. The DB write is authoritative and always applied
	// even if the Hydra delete fails — see the comment on Revoke below.
	// tenantID scopes the lookup: a service_client belonging to a different
	// tenant is reported not-found, never revoked.
	Revoke(tenantID, id uuid.UUID) error
}

type service struct {
	db          *gorm.DB
	logger      *zap.Logger
	hydraClient *hydra.Client
}

func NewService(db *gorm.DB, logger *zap.Logger, hydraClient *hydra.Client) Service {
	return &service{db: db, logger: logger, hydraClient: hydraClient}
}

func (s *service) Create(tenantID uuid.UUID, req *CreateServiceClientRequest) (*ServiceClient, *ClientCredentials, error) {
	for _, scope := range req.Scopes {
		if !allowedScopes[scope] {
			return nil, nil, apierror.NewPublic(fmt.Sprintf("scope %q is not allowed for service clients", scope))
		}
	}

	var tenantExists bool
	if err := s.db.Raw("SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND active = true)", tenantID).Scan(&tenantExists).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to verify tenant: %w", err)
	}
	if !tenantExists {
		return nil, nil, apierror.NewPublic("tenant not found or inactive")
	}

	clientID, err := generateHydraClientID()
	if err != nil {
		return nil, nil, err
	}
	clientSecret, err := generateHydraClientSecret()
	if err != nil {
		return nil, nil, err
	}

	sc := &ServiceClient{
		TenantID:      tenantID,
		HydraClientID: clientID,
		Name:          req.Name,
		GrantedScopes: req.Scopes,
	}
	if err := s.db.Create(sc).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to create service client: %w", err)
	}

	_, err = s.hydraClient.CreateOAuth2Client(&hydra.OAuth2Client{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              req.Name,
		GrantTypes:              []string{"client_credentials"},
		ResponseTypes:           []string{},
		Scope:                   strings.Join(req.Scopes, " "),
		TokenEndpointAuthMethod: "client_secret_post",
	})
	if err != nil {
		// Hydra registration failed — roll back the DB row so a retry with
		// the same name doesn't leave an orphaned mapping to a Hydra client
		// that doesn't exist. Mirrors client.Service.Create's rollback
		// direction for the same reason (pkg/client/service.go).
		s.db.Delete(sc)
		s.logger.Error("Failed to register service client in Hydra, rolled back DB row",
			zap.Error(err), zap.String("hydra_client_id", clientID), zap.String("tenant_id", tenantID.String()))
		return nil, nil, fmt.Errorf("failed to register service client in Hydra: %w", err)
	}

	s.logger.Info("Service client created", zap.String("id", sc.ID.String()),
		zap.String("hydra_client_id", clientID), zap.String("tenant_id", tenantID.String()))

	return sc, &ClientCredentials{ClientID: clientID, ClientSecret: clientSecret}, nil
}

func (s *service) GetByHydraClientID(hydraClientID string) (*ServiceClient, error) {
	var sc ServiceClient
	if err := s.db.Where("hydra_client_id = ?", hydraClientID).First(&sc).Error; err != nil {
		return nil, fmt.Errorf("service client not found: %w", err)
	}
	return &sc, nil
}

func (s *service) ListByTenant(tenantID uuid.UUID, limit, offset int) ([]*ServiceClient, int64, error) {
	var scs []*ServiceClient
	var total int64

	if err := s.db.Model(&ServiceClient{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count service clients: %w", err)
	}

	if err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Limit(limit).Offset(offset).Find(&scs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list service clients: %w", err)
	}

	return scs, total, nil
}

func (s *service) Revoke(tenantID, id uuid.UUID) error {
	var sc ServiceClient
	if err := s.db.Where("tenant_id = ?", tenantID).First(&sc, "id = ?", id).Error; err != nil {
		return fmt.Errorf("service client not found: %w", err)
	}

	if err := s.db.Model(&sc).Update("revoked_at", gorm.Expr("NOW()")).Error; err != nil {
		return fmt.Errorf("failed to revoke service client: %w", err)
	}

	// Best-effort: the DB revoked_at write above is what actually blocks
	// every future request (GetByHydraClientID's caller checks IsRevoked()
	// on every introspection), so a Hydra-side failure here does not leave
	// the credential usable — it only means Hydra could still issue a new
	// token for a client_id this service will now always reject.
	if err := s.hydraClient.DeleteOAuth2Client(sc.HydraClientID); err != nil {
		s.logger.Warn("Failed to delete Hydra client during revoke (DB revoke still applied)",
			zap.Error(err), zap.String("hydra_client_id", sc.HydraClientID))
	}

	return nil
}

func generateHydraClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed generating service client id: %w", err)
	}
	return fmt.Sprintf("authway_svc_%s", base64.URLEncoding.EncodeToString(b)[:22]), nil
}

func generateHydraClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed generating service client secret: %w", err)
	}
	return strings.ReplaceAll(base64.URLEncoding.EncodeToString(b), "=", ""), nil
}
