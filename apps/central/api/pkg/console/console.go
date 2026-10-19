// Package console holds the accounts that administer Authway and what each
// may administer.
//
// A console account is a user of one tenant, the system tenant
// (tenants.is_system): it signs in like any user — password, second factor,
// providers, invitation — and the console presents its access token to the
// administration API. A membership says what the account administers:
// RoleOperator every tenant, as the admin key does, or RoleOwner/RoleAdmin one
// tenant.
package console

import (
	"context"
	"errors"
	"time"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tenantscope"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Audience is the audience of the access tokens the administration API takes
// from a console account. The console's client is registered with it in the
// system tenant; a token without it — a token any other application of that
// tenant holds — administers nothing.
const Audience = client.ConsoleAudience

// RoleOperator administers every tenant.
const RoleOperator = "operator"

// Membership is what one console account administers.
type Membership struct {
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID  `json:"user_id" gorm:"type:uuid;not null"`
	TenantID  *uuid.UUID `json:"tenant_id" gorm:"type:uuid"`
	Role      string     `json:"role" gorm:"size:16;not null"`
	CreatedAt time.Time  `json:"created_at" gorm:"not null"`
}

// TableName is the table of memberships.
func (Membership) TableName() string { return "console_memberships" }

var (
	// ErrNoSystemTenant: no tenant has been chosen for console accounts.
	ErrNoSystemTenant = errors.New("console: no system tenant")
	// ErrNotConsoleAccount: the user is not an account of the system tenant.
	ErrNotConsoleAccount = errors.New("console: not an account of the system tenant")
	// ErrUnknownTenant: the membership names a tenant that does not exist.
	ErrUnknownTenant = errors.New("console: unknown tenant")
	// ErrInvalidRole: the role does not fit the membership (operator without
	// a tenant, owner or admin with one).
	ErrInvalidRole = errors.New("console: invalid role")
	// ErrSystemTenantInUse: console accounts of the current system tenant
	// still hold memberships, so it cannot be replaced.
	ErrSystemTenantInUse = errors.New("console: the system tenant has members")
	// ErrNotFound: no such membership.
	ErrNotFound = errors.New("console: membership not found")
	// ErrSystemTenantMembership: the system tenant is administered by
	// operators only. Its users are the console accounts; whoever could
	// change them — an address, a password, a suspension — could take an
	// operator's account.
	ErrSystemTenantMembership = errors.New("console: the system tenant is administered by operators only")
)

// Service reads and changes console memberships.
type Service struct {
	db *gorm.DB
}

// NewService returns a Service on db.
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// SystemTenant returns the tenant whose accounts sign in to the console.
func (s *Service) SystemTenant(ctx context.Context) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := s.db.WithContext(ctx).Where("is_system").Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNoSystemTenant
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SystemTenantID returns the id of the tenant whose accounts sign in to the
// console.
func (s *Service) SystemTenantID(ctx context.Context) (uuid.UUID, error) {
	t, err := s.SystemTenant(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}

// SetSystemTenant makes the tenant the one whose accounts sign in to the
// console. Replacing a system tenant whose accounts hold memberships is
// refused — those accounts would keep memberships nobody can use.
func (s *Service) SetSystemTenant(ctx context.Context, id uuid.UUID) (*tenant.Tenant, error) {
	var chosen tenant.Tenant
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&chosen).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUnknownTenant
			}
			return err
		}
		if chosen.IsSystem {
			return nil
		}
		var members int64
		if err := tx.Model(&Membership{}).Count(&members).Error; err != nil {
			return err
		}
		if members > 0 {
			return ErrSystemTenantInUse
		}
		if err := tx.Model(&tenant.Tenant{}).Where("is_system").Update("is_system", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&chosen).Update("is_system", true).Error; err != nil {
			return err
		}
		chosen.IsSystem = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &chosen, nil
}

// MembershipsOf returns the memberships of a console account.
func (s *Service) MembershipsOf(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	var ms []Membership
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at").Find(&ms).Error
	return ms, err
}

// ScopeOf turns an account's memberships into what it administers.
func ScopeOf(ms []Membership) tenantscope.Scope {
	tenants := map[uuid.UUID]tenantscope.Role{}
	for _, m := range ms {
		if m.Role == RoleOperator {
			return tenantscope.All()
		}
		if m.TenantID != nil {
			tenants[*m.TenantID] = tenantscope.Role(m.Role)
		}
	}
	return tenantscope.Only(tenants)
}

// List returns the memberships of the given tenants, or, with all set, every
// membership — operators included.
func (s *Service) List(ctx context.Context, tenants []uuid.UUID, all bool) ([]Membership, error) {
	q := s.db.WithContext(ctx).Order("created_at")
	if !all {
		if len(tenants) == 0 {
			return []Membership{}, nil
		}
		q = q.Where("tenant_id IN ?", tenants)
	}
	var ms []Membership
	err := q.Find(&ms).Error
	return ms, err
}

// Get returns a membership by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Membership, error) {
	var m Membership
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Put gives a console account a role: in the tenant, or, with tenantID nil,
// the operator role. An account holds one role per tenant; putting another
// replaces it. created reports whether the membership is new.
func (s *Service) Put(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID, role string) (m *Membership, created bool, err error) {
	switch {
	case tenantID == nil && role != RoleOperator,
		tenantID != nil && role != string(tenantscope.RoleOwner) && role != string(tenantscope.RoleAdmin):
		return nil, false, ErrInvalidRole
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var system tenant.Tenant
		if err := tx.Where("is_system").Take(&system).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoSystemTenant
			}
			return err
		}
		var accounts int64
		if err := tx.Table("users").Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", userID, system.ID).Count(&accounts).Error; err != nil {
			return err
		}
		if accounts == 0 {
			return ErrNotConsoleAccount
		}
		if tenantID != nil && *tenantID == system.ID {
			return ErrSystemTenantMembership
		}
		if tenantID != nil {
			var tenants int64
			if err := tx.Model(&tenant.Tenant{}).Where("id = ?", *tenantID).Count(&tenants).Error; err != nil {
				return err
			}
			if tenants == 0 {
				return ErrUnknownTenant
			}
		}
		q := tx.Where("user_id = ?", userID)
		if tenantID == nil {
			q = q.Where("tenant_id IS NULL")
		} else {
			q = q.Where("tenant_id = ?", *tenantID)
		}
		var existing Membership
		switch err := q.Take(&existing).Error; {
		case err == nil:
			if err := tx.Model(&existing).Update("role", role).Error; err != nil {
				return err
			}
			existing.Role = role
			m = &existing
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}
		m = &Membership{ID: uuid.New(), UserID: userID, TenantID: tenantID, Role: role, CreatedAt: time.Now().UTC()}
		created = true
		return tx.Create(m).Error
	})
	if err != nil {
		return nil, false, err
	}
	return m, created, nil
}

// Remove deletes a membership.
func (s *Service) Remove(ctx context.Context, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&Membership{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
