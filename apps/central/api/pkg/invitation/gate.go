package invitation

import (
	"fmt"
	"time"

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
