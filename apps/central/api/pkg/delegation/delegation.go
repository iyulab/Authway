// Package delegation keeps what users let clients do for them while they are
// not there.
//
// A user grants a delegation by consenting to a client's `delegation` scope:
// the client may then act for them, with the other scopes of that consent,
// until the user (or an administrator) revokes it. The client receives a
// handle for the delegation in the ID token of that consent and exchanges it,
// authenticating as itself, for a short-lived access token whose subject is
// the user and whose actor is the client (RFC 8693).
//
// A delegation is in force only while its user and its client exist: both are
// soft-deleted, so a removed account or client leaves its delegations behind,
// and every read here leaves those out.
package delegation

import (
	"context"
	"errors"
	"time"

	"authway/apps/central/api/pkg/tokenhash"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// MaxPerClient is how many delegations in force a user may have granted one
// client. Each consent to the delegation scope grants a new one (as each
// consent to offline_access issues a new refresh token); past this many the
// oldest is revoked, so a client that asks every time cannot pile them up.
const MaxPerClient = 20

// ErrNotFound reports a delegation that does not exist, is not in force, or is
// not the caller's to see.
var ErrNotFound = errors.New("delegation not found")

// Grant is a stored delegation. Only the SHA-256 of its handle is kept.
type Grant struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey"`
	TenantID   uuid.UUID      `gorm:"type:uuid;not null"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null"`
	ClientID   uuid.UUID      `gorm:"type:uuid;not null"`
	Scopes     pq.StringArray `gorm:"type:text[];not null"`
	HandleHash string         `gorm:"size:64;not null"`
	CreatedAt  time.Time      `gorm:"not null"`
	ExpiresAt  *time.Time     `gorm:""`
	RevokedAt  *time.Time     `gorm:""`
}

// TableName names Grant's table.
func (Grant) TableName() string { return "delegation_grants" }

// View is a delegation as the API shows it (Delegation in the contract):
// the client by its OAuth client_id and name.
type View struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"-"`
	UserID     uuid.UUID  `json:"user_id"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// Filter narrows a tenant's delegations. Zero values do not narrow.
type Filter struct {
	UserID   uuid.UUID
	ClientID string // OAuth client_id
}

// Service grants, lists and revokes delegations.
type Service interface {
	// Grant records a delegation userID grants the client with row id
	// clientID, with scopes as its ceiling, and returns its handle — the only
	// time the handle exists outside the client.
	Grant(ctx context.Context, tenantID, userID, clientID uuid.UUID, scopes []string) (handle string, g *Grant, err error)
	// List returns a tenant's delegations in force, newest first.
	List(ctx context.Context, tenantID uuid.UUID, f Filter) ([]View, error)
	// ListForUser returns the delegations in force a user granted, newest
	// first.
	ListForUser(ctx context.Context, userID uuid.UUID) ([]View, error)
	// Find returns one delegation in force.
	Find(ctx context.Context, id uuid.UUID) (*View, error)
	// Revoke ends a delegation in force.
	Revoke(ctx context.Context, id uuid.UUID) error
}

type service struct {
	db  *gorm.DB
	now func() time.Time
}

// NewService returns a Service over db.
func NewService(db *gorm.DB) Service {
	return &service{db: db, now: time.Now}
}

func (s *service) Grant(ctx context.Context, tenantID, userID, clientID uuid.UUID, scopes []string) (string, *Grant, error) {
	handle, err := tokenhash.Generate()
	if err != nil {
		return "", nil, err
	}
	g := &Grant{
		ID:         uuid.New(),
		TenantID:   tenantID,
		UserID:     userID,
		ClientID:   clientID,
		Scopes:     pq.StringArray(append([]string{}, scopes...)),
		HandleHash: tokenhash.Hash(handle),
		CreatedAt:  s.now().UTC(),
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(g).Error; err != nil {
			return err
		}
		// Past MaxPerClient, the oldest of this user's delegations to this
		// client stop being in force.
		return tx.Exec(`
			UPDATE delegation_grants SET revoked_at = ?
			 WHERE id IN (
			   SELECT id FROM delegation_grants
			    WHERE user_id = ? AND client_id = ? AND revoked_at IS NULL
			    ORDER BY created_at DESC, id DESC
			   OFFSET ?)`, g.CreatedAt, userID, clientID, MaxPerClient).Error
	})
	if err != nil {
		return "", nil, err
	}
	return handle, g, nil
}

// inForce selects the delegations in force, with their client, of users and
// clients that still exist.
func (s *service) inForce(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).
		Table("delegation_grants AS d").
		Select("d.id, d.tenant_id, d.user_id, c.client_id, c.name AS client_name, d.scopes, d.created_at, d.expires_at").
		Joins("JOIN clients c ON c.id = d.client_id AND c.deleted_at IS NULL").
		Joins("JOIN users u ON u.id = d.user_id AND u.deleted_at IS NULL").
		Where("d.revoked_at IS NULL AND (d.expires_at IS NULL OR d.expires_at > ?)", s.now().UTC())
}

type row struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	UserID     uuid.UUID
	ClientID   string
	ClientName string
	Scopes     pq.StringArray `gorm:"type:text[]"`
	CreatedAt  time.Time
	ExpiresAt  *time.Time
}

func (r row) view() View {
	scopes := []string(r.Scopes)
	if scopes == nil {
		scopes = []string{}
	}
	return View{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, ClientID: r.ClientID, ClientName: r.ClientName, Scopes: scopes, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt}
}

func (s *service) List(ctx context.Context, tenantID uuid.UUID, f Filter) ([]View, error) {
	q := s.inForce(ctx).Where("d.tenant_id = ?", tenantID)
	if f.UserID != uuid.Nil {
		q = q.Where("d.user_id = ?", f.UserID)
	}
	if f.ClientID != "" {
		q = q.Where("c.client_id = ?", f.ClientID)
	}
	return s.views(q)
}

func (s *service) ListForUser(ctx context.Context, userID uuid.UUID) ([]View, error) {
	return s.views(s.inForce(ctx).Where("d.user_id = ?", userID))
}

func (s *service) views(q *gorm.DB) ([]View, error) {
	var rows []row
	if err := q.Order("d.created_at DESC, d.id DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, r := range rows {
		views = append(views, r.view())
	}
	return views, nil
}

func (s *service) Find(ctx context.Context, id uuid.UUID) (*View, error) {
	var rows []row
	if err := s.inForce(ctx).Where("d.id = ?", id).Limit(1).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	v := rows[0].view()
	return &v, nil
}

func (s *service) Revoke(ctx context.Context, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Model(&Grant{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", s.now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
