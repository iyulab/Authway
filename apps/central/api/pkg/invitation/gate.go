package invitation

import (
	"fmt"
	"time"

	"authway/apps/central/api/pkg/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Gate answers the one question the auto-provisioning paths need: has this
// address been invited into this tenant?
//
// It is deliberately separate from Service. Enforcing invitation-only onboarding
// needs nothing but a database read, while Service needs mail delivery, the
// tenant service and a base URL — and social login is constructed long before
// any of those exist. Depending on the full service here would mean reordering
// startup to satisfy a single SELECT.
type Gate struct {
	db *gorm.DB
}

func NewGate(db *gorm.DB) *Gate {
	return &Gate{db: db}
}

// HasValidInvitation reports whether (tenantID, email) has a pending, unexpired
// invitation. Errors are returned rather than swallowed so callers can fail
// closed — treating an unreadable invitations table as "no policy" would
// silently restore self-registration.
func (g *Gate) HasValidInvitation(tenantID uuid.UUID, email string) (bool, error) {
	var count int64
	if err := g.db.Model(&Invitation{}).
		Where("tenant_id = ? AND email = ? AND status = ? AND expires_at > ?",
			tenantID, email, StatusPending, time.Now()).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check invitations: %w", err)
	}
	return count > 0, nil
}

// MayProvision reports whether a first-time sign-in for (tenantID, email) may
// create a new account: the address is in a domain the tenant allows accounts
// for (allowed_domains — any, when it names none), and either the tenant's
// signup_mode is "open" or the address holds a valid invitation. This
// is the single policy decision every auto-provisioning path (social login,
// magic link) must consult — previously each caller re-implemented it as a
// direct HasValidInvitation check, which had no way to honor a per-tenant
// open-signup setting without editing every call site.
func (g *Gate) MayProvision(tenantID uuid.UUID, email string) (bool, error) {
	settings, err := g.settings(tenantID)
	if err != nil {
		return false, err
	}
	if !settings.AllowsAddress(email) {
		return false, nil
	}
	if settings.SignupMode == tenant.SignupModeOpen {
		return true, nil
	}
	return g.HasValidInvitation(tenantID, email)
}

// settings reads the tenant's settings directly — Gate depends on nothing but
// a database read (see the type doc above), and a tenant lookup by ID is the
// same shape of dependency HasValidInvitation already has. A tenant that is
// not there has the settings of one that set nothing: invitation only.
func (g *Gate) settings(tenantID uuid.UUID) (tenant.TenantSettings, error) {
	var row struct{ Settings tenant.TenantSettings }
	if err := g.db.Raw(`SELECT settings FROM tenants WHERE id = ?`, tenantID).Scan(&row).Error; err != nil {
		return tenant.TenantSettings{}, fmt.Errorf("failed to read tenant sign-up settings: %w", err)
	}
	return row.Settings, nil
}
