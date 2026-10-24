package console

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"authway/apps/central/api/pkg/tenant"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SupportAccessDuration is how long support access stays open. It is not
// chosen per entry: an operator who needs longer starts again, and the new
// entry carries its own reason.
const SupportAccessDuration = time.Hour

// SupportReasonMaxLength bounds the reason an operator gives.
const SupportReasonMaxLength = 500

// Who ended support access.
const (
	SupportEndedByOperator = "operator"
	SupportEndedByExpiry   = "expired"
)

// SupportAccess is an operator's entry into one tenant's contents.
type SupportAccess struct {
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID  `json:"user_id" gorm:"type:uuid;not null"`
	TenantID  uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null"`
	Reason    string     `json:"reason" gorm:"size:500;not null"`
	StartedAt time.Time  `json:"started_at" gorm:"not null"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	EndedBy   *string    `json:"ended_by,omitempty" gorm:"size:16"`
}

// TableName is the table of support access.
func (SupportAccess) TableName() string { return "console_support_access" }

var (
	// ErrSupportReason: the reason is empty or too long.
	ErrSupportReason = errors.New("console: support access needs a reason of 1 to 500 characters")
	// ErrNotOperator: only an operator opens support access.
	ErrNotOperator = errors.New("console: only an operator opens support access")
	// ErrSupportNotFound: no such open support access of this account.
	ErrSupportNotFound = errors.New("console: support access not found")
)

// IsOperator reports whether the memberships include the operator role.
func IsOperator(ms []Membership) bool {
	for _, m := range ms {
		if m.Role == RoleOperator {
			return true
		}
	}
	return false
}

// openAt narrows a query to support access open at now.
func openAt(q *gorm.DB, now time.Time) *gorm.DB {
	return q.Where("ended_at IS NULL AND expires_at > ?", now)
}

// OpenSupportTenants returns the tenants the account holds open support
// access to, each with the entry that opened it (the newest, when there are
// several).
func (s *Service) OpenSupportTenants(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	open, err := s.OpenSupport(ctx, userID)
	if err != nil {
		return nil, err
	}
	byTenant := make(map[uuid.UUID]uuid.UUID, len(open))
	for _, a := range open {
		if _, seen := byTenant[a.TenantID]; !seen {
			byTenant[a.TenantID] = a.ID
		}
	}
	return byTenant, nil
}

// OpenSupport returns the account's open support access, newest first.
func (s *Service) OpenSupport(ctx context.Context, userID uuid.UUID) ([]SupportAccess, error) {
	var open []SupportAccess
	err := openAt(s.db.WithContext(ctx), time.Now()).
		Where("user_id = ?", userID).Order("started_at DESC").Find(&open).Error
	return open, err
}

// StartSupport opens support access to the tenant for an operator's account.
func (s *Service) StartSupport(ctx context.Context, userID, tenantID uuid.UUID, reason string) (*SupportAccess, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > SupportReasonMaxLength {
		return nil, ErrSupportReason
	}
	ms, err := s.MembershipsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !IsOperator(ms) {
		return nil, ErrNotOperator
	}
	var t tenant.Tenant
	if err := s.db.WithContext(ctx).Where("id = ?", tenantID).Take(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnknownTenant
		}
		return nil, err
	}
	now := time.Now().UTC()
	a := &SupportAccess{
		ID:        uuid.New(),
		UserID:    userID,
		TenantID:  tenantID,
		Reason:    reason,
		StartedAt: now,
		ExpiresAt: now.Add(SupportAccessDuration),
	}
	if err := s.db.WithContext(ctx).Create(a).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// EndSupport ends the account's open support access id.
func (s *Service) EndSupport(ctx context.Context, userID, id uuid.UUID) (*SupportAccess, error) {
	now := time.Now().UTC()
	by := SupportEndedByOperator
	var ended SupportAccess
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := openAt(tx.Model(&SupportAccess{}), now).
			Where("id = ? AND user_id = ?", id, userID).
			Updates(map[string]any{"ended_at": now, "ended_by": by})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrSupportNotFound
		}
		return tx.Where("id = ?", id).Take(&ended).Error
	})
	if err != nil {
		return nil, err
	}
	return &ended, nil
}

// CloseExpiredSupport records the end of every support access that expired
// while open and returns them, so each can be written to its tenant's audit
// log.
func (s *Service) CloseExpiredSupport(ctx context.Context, now time.Time) ([]SupportAccess, error) {
	var closed []SupportAccess
	// One statement: an entry its operator ends at the same moment is
	// recorded once, by whichever write comes first.
	err := s.db.WithContext(ctx).Model(&closed).Clauses(clause.Returning{}).
		Where("ended_at IS NULL AND expires_at <= ?", now).
		Updates(map[string]any{"ended_at": now, "ended_by": SupportEndedByExpiry}).Error
	return closed, err
}
