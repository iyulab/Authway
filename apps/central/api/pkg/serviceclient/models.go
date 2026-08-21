// Package serviceclient implements tenant-scoped M2M provisioning
// credentials for consumer apps that need to programmatically create OAuth
// clients without full admin access.
package serviceclient

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// ServiceClient maps a Hydra client_credentials OAuth2Client to exactly one
// Authway tenant with an explicit scope allowlist. Hydra is the credential
// store (client_id/client_secret); this row exists only for the tenant +
// scope mapping Hydra has no concept of.
type ServiceClient struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID      uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	HydraClientID string         `json:"hydra_client_id" gorm:"column:hydra_client_id;uniqueIndex;not null"`
	Name          string         `json:"name" gorm:"not null"`
	GrantedScopes pq.StringArray `json:"granted_scopes" gorm:"type:text[];column:granted_scopes;not null;default:'{}'"`
	RevokedAt     *time.Time     `json:"revoked_at" gorm:"column:revoked_at"`
	CreatedAt     time.Time      `json:"created_at"`
}

// TableName pins the table name explicitly — GORM's pluralization of
// "ServiceClient" already produces "service_clients", but every other model
// in this codebase pins it explicitly rather than relying on inference.
func (ServiceClient) TableName() string {
	return "service_clients"
}

func (sc *ServiceClient) BeforeCreate(tx *gorm.DB) error {
	if sc.ID == uuid.Nil {
		sc.ID = uuid.New()
	}
	return nil
}

// IsRevoked reports whether this credential has been revoked.
func (sc *ServiceClient) IsRevoked() bool {
	return sc.RevokedAt != nil
}

// HasScope reports whether this credential was granted scope.
func (sc *ServiceClient) HasScope(scope string) bool {
	for _, s := range sc.GrantedScopes {
		if s == scope {
			return true
		}
	}
	return false
}
