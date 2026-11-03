package user

import (
	"time"

	"github.com/google/uuid"
)

// LinkedIdentity is one provider account an account signs in with, as a row of
// user_identities. Within a tenant a provider account belongs to one account
// (unique index uq_user_identities_tenant_provider_subject).
//
// The rows are written next to the provider columns on users, which are still
// what is read; a later release reconciles the two and reads the rows.
type LinkedIdentity struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID uuid.UUID `gorm:"type:uuid;not null"`
	UserID   uuid.UUID `gorm:"type:uuid;not null"`
	Provider string    `gorm:"not null"`
	Subject  string    `gorm:"not null"`
	// EmailAtLink is the address the provider reported when the account was
	// linked. The account's own address does not change with it.
	EmailAtLink string    `gorm:"not null"`
	LinkedAt    time.Time `gorm:"not null"`
}

// TableName is the table the identities live in.
func (LinkedIdentity) TableName() string { return "user_identities" }
