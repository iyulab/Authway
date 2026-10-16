package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"authway/apps/central/api/pkg/tokenhash"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Approval states. Expiry and consumption are read from expires_at and
// consumed_at, not stored as a state.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalDenied   = "denied"
)

// ApprovalPollInterval is how often a client may ask whether an approval was
// decided (the interval of the answer to an approval request).
const ApprovalPollInterval = 5 * time.Second

// Approval is a stored approval request: a client acting for a user (Grant)
// asked the user to approve one action.
type Approval struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey"`
	TenantID             uuid.UUID      `gorm:"type:uuid;not null"`
	UserID               uuid.UUID      `gorm:"type:uuid;not null"`
	ClientID             uuid.UUID      `gorm:"type:uuid;not null"`
	DelegationID         uuid.UUID      `gorm:"type:uuid;not null"`
	AuthReqHash          string         `gorm:"size:64;not null"`
	BindingMessage       string         `gorm:"size:200;not null"`
	Scopes               pq.StringArray `gorm:"type:text[];not null"`
	AuthorizationDetails *string        `gorm:"type:jsonb"`
	Status               string         `gorm:"size:16;not null"`
	CreatedAt            time.Time      `gorm:"not null"`
	ExpiresAt            time.Time      `gorm:"not null"`
	DecidedAt            *time.Time
	ConsumedAt           *time.Time
	LastPolledAt         *time.Time
}

// TableName names Approval's table.
func (Approval) TableName() string { return "approval_requests" }

// ErrApprovalNotFound reports an approval request that does not exist or is
// not the caller's.
var ErrApprovalNotFound = errors.New("approval request not found")

// ErrApprovalSettled reports a decision on a request that is no longer
// pending (decided or expired).
var ErrApprovalSettled = errors.New("approval request is no longer pending")

// ApprovalView is an approval request as its user sees it.
type ApprovalView struct {
	ID                   uuid.UUID       `json:"id"`
	ClientID             string          `json:"client_id"`
	ClientName           string          `json:"client_name"`
	BindingMessage       string          `json:"binding_message"`
	Scopes               []string        `json:"scopes"`
	AuthorizationDetails json.RawMessage `json:"authorization_details,omitempty"`
	Status               string          `json:"status"`
	CreatedAt            time.Time       `json:"created_at"`
	ExpiresAt            time.Time       `json:"expires_at"`
	UserID               uuid.UUID       `json:"-"`
}

// Approvals keeps approval requests.
type Approvals struct {
	db  *gorm.DB
	now func() time.Time
}

// NewApprovals returns the approval request store over db.
func NewApprovals(db *gorm.DB) *Approvals { return &Approvals{db: db, now: time.Now} }

// Create records a request and returns its polling secret.
func (a *Approvals) Create(ctx context.Context, d *Standing, clientRow uuid.UUID, binding string, scopes []string, details json.RawMessage, ttl time.Duration) (string, *Approval, error) {
	secret, err := tokenhash.Generate()
	if err != nil {
		return "", nil, err
	}
	now := a.now().UTC()
	r := &Approval{
		ID:             uuid.New(),
		TenantID:       d.TenantID,
		UserID:         d.UserID,
		ClientID:       clientRow,
		DelegationID:   d.ID,
		AuthReqHash:    tokenhash.Hash(secret),
		BindingMessage: binding,
		Scopes:         pq.StringArray(append([]string{}, scopes...)),
		Status:         ApprovalPending,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
	}
	if len(details) > 0 {
		d := string(details)
		r.AuthorizationDetails = &d
	}
	if err := a.db.WithContext(ctx).Create(r).Error; err != nil {
		return "", nil, err
	}
	return secret, r, nil
}

// ForUser returns a request of the user's, with its client.
func (a *Approvals) ForUser(ctx context.Context, userID, id uuid.UUID) (*ApprovalView, error) {
	var rows []struct {
		Approval
		OAuthClientID string `gorm:"column:oauth_client_id"`
		ClientName    string `gorm:"column:client_name"`
	}
	err := a.db.WithContext(ctx).Table("approval_requests AS r").
		Select("r.*, c.client_id AS oauth_client_id, c.name AS client_name").
		Joins("JOIN clients c ON c.id = r.client_id AND c.deleted_at IS NULL").
		Where("r.id = ? AND r.user_id = ?", id, userID).
		Limit(1).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrApprovalNotFound
	}
	r := rows[0]
	v := &ApprovalView{
		ID: r.ID, ClientID: r.OAuthClientID, ClientName: r.ClientName, BindingMessage: r.BindingMessage,
		Scopes: append([]string{}, r.Scopes...), Status: a.status(&r.Approval), CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, UserID: r.UserID,
	}
	if r.AuthorizationDetails != nil {
		v.AuthorizationDetails = json.RawMessage(*r.AuthorizationDetails)
	}
	return v, nil
}

// status is a request's state as shown: expired once past its expiry while
// pending, consumed once its token was issued.
func (a *Approvals) status(r *Approval) string {
	switch {
	case r.ConsumedAt != nil:
		return "consumed"
	case r.Status == ApprovalPending && !a.now().Before(r.ExpiresAt):
		return "expired"
	}
	return r.Status
}

// Decide approves or denies a pending request of the user's.
func (a *Approvals) Decide(ctx context.Context, userID, id uuid.UUID, approve bool) error {
	status := ApprovalDenied
	if approve {
		status = ApprovalApproved
	}
	now := a.now().UTC()
	res := a.db.WithContext(ctx).Model(&Approval{}).
		Where("id = ? AND user_id = ? AND status = ? AND expires_at > ?", id, userID, ApprovalPending, now).
		Updates(map[string]any{"status": status, "decided_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		a.db.WithContext(ctx).Model(&Approval{}).Where("id = ? AND user_id = ?", id, userID).Count(&n)
		if n == 0 {
			return ErrApprovalNotFound
		}
		return ErrApprovalSettled
	}
	return nil
}

// Polled is what a poll finds.
type Polled struct {
	Approval *Approval
	// TooSoon is true when the client asked again within the interval.
	TooSoon bool
	// State is "pending", "approved", "denied", "expired" or "consumed".
	State string
}

// Poll finds the request a polling secret names, for the client with row id
// clientRow, and records when it was asked.
func (a *Approvals) Poll(ctx context.Context, secret string, clientRow uuid.UUID) (*Polled, error) {
	var r Approval
	err := a.db.WithContext(ctx).Where("auth_req_hash = ? AND client_id = ?", tokenhash.Hash(secret), clientRow).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	now := a.now().UTC()
	p := &Polled{Approval: &r, State: a.status(&r)}
	if r.LastPolledAt != nil && now.Sub(*r.LastPolledAt) < ApprovalPollInterval {
		p.TooSoon = true
	}
	a.db.WithContext(ctx).Model(&Approval{}).Where("id = ?", r.ID).Update("last_polled_at", now)
	return p, nil
}

// Consume marks an approved request as having had its token issued, once.
// It reports false when the request is not approved, not the client's, or
// was consumed already.
func (a *Approvals) Consume(ctx context.Context, id, delegationID uuid.UUID) (*Approval, bool, error) {
	now := a.now().UTC()
	res := a.db.WithContext(ctx).Model(&Approval{}).
		Where("id = ? AND delegation_id = ? AND status = ? AND consumed_at IS NULL", id, delegationID, ApprovalApproved).
		Update("consumed_at", now)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	var r Approval
	if err := a.db.WithContext(ctx).Where("id = ?", id).First(&r).Error; err != nil {
		return nil, false, err
	}
	return &r, true, nil
}
