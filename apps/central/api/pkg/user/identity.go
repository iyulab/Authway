package user

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// LinkedIdentity is one provider account an account signs in with, as a row of
// user_identities. An account may have several, at one provider or at several;
// within a tenant a provider account belongs to one account (unique index
// uq_user_identities_tenant_provider_subject).
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

// providerOrder is the fixed order in which an account's providers are listed.
var providerOrder = []string{"google", "github", "microsoft", "apple"}

// LinkedProviders names the sign-in providers the account is linked to, in a
// fixed order, each once. It reads Identities, which the user service fills in
// with every account it returns.
func (u *User) LinkedProviders() []string {
	has := map[string]bool{}
	for _, identity := range u.Identities {
		has[identity.Provider] = true
	}
	var linked []string
	for _, p := range providerOrder {
		if has[p] {
			linked = append(linked, p)
		}
	}
	return linked
}

// SubjectAt returns the provider account the account signs in with at
// provider — the oldest, when it has several — or "".
func (u *User) SubjectAt(provider string) string {
	for _, identity := range u.Identities {
		if identity.Provider == provider {
			return identity.Subject
		}
	}
	return ""
}

// NoteLinked records on the in-memory account a link just stored, so the
// account the caller holds matches what a fresh read would return.
func (u *User) NoteLinked(provider, subject string) {
	for _, identity := range u.Identities {
		if identity.Provider == provider && identity.Subject == subject {
			return
		}
	}
	u.Identities = append(u.Identities, LinkedIdentity{TenantID: u.TenantID, UserID: u.ID, Provider: provider, Subject: subject, EmailAtLink: u.Email})
}

// withIdentities fills in Identities for each of the accounts, oldest first,
// in one query.
func (s *service) withIdentities(users ...*User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	var rows []LinkedIdentity
	if err := s.db.Where("user_id IN ?", ids).Order("linked_at, id").Find(&rows).Error; err != nil {
		return fmt.Errorf("failed to load sign-in identities: %w", err)
	}
	byUser := make(map[uuid.UUID][]LinkedIdentity, len(users))
	for _, row := range rows {
		byUser[row.UserID] = append(byUser[row.UserID], row)
	}
	for _, u := range users {
		u.Identities = byUser[u.ID]
	}
	return nil
}

// loaded fills in the identities of an account the service is about to return.
func (s *service) loaded(u *User) (*User, error) {
	if err := s.withIdentities(u); err != nil {
		return nil, err
	}
	return u, nil
}

// ProviderLink is a provider account to link to an account.
type ProviderLink struct {
	Provider string
	// Subject is the provider's own identifier for the account.
	Subject string
	// Email is the address the provider reported; the account's own address is
	// recorded when it is empty.
	Email string
	// VerifiesAddress marks the account's address verified: the link is made on
	// an address the provider has verified and that is the account's own.
	VerifiesAddress bool
}
