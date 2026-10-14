// Package terms keeps the documents a tenant's users accept when they sign
// in — terms of service, a privacy policy — and what each user accepted.
//
// A tenant names its documents by kind, one current version per kind. The
// documents themselves live wherever the tenant's applications publish them;
// Authway knows each version's address and records acceptances. A sign-in
// owes the current version of every kind its user has not accepted yet (see
// Service.Owed). Optional consents — ones a user may refuse and still sign
// in — are the application's own.
package terms

import (
	"time"

	"github.com/google/uuid"
)

// Document is one version of one of a tenant's documents. The current version
// of a kind is the one with no RetiredAt.
type Document struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null"`
	Kind        string     `gorm:"not null"`
	Version     string     `gorm:"not null"`
	URL         string     `gorm:"column:url;not null"`
	Title       *string    `gorm:""`
	PublishedAt time.Time  `gorm:"not null"`
	RetiredAt   *time.Time `gorm:""`
}

// TableName names Document's table.
func (Document) TableName() string { return "tenant_terms" }

// Acceptance records that a user accepted a document version.
type Acceptance struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID `gorm:"type:uuid;not null"`
	TermsID    uuid.UUID `gorm:"type:uuid;not null"`
	ClientID   *string   `gorm:""`
	AcceptedAt time.Time `gorm:"not null"`
}

// TableName names Acceptance's table.
func (Acceptance) TableName() string { return "user_terms_acceptances" }

// DocumentView is a document as the API shows it (TermsDocument in the
// contract). PublishedAt is left out of what a sign-in screen is shown.
type DocumentView struct {
	Kind        string     `json:"kind"`
	Version     string     `json:"version"`
	URL         string     `json:"url"`
	Title       string     `json:"title,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

// View is d as an administrator sees it.
func (d Document) View() DocumentView {
	v := d.Shown()
	published := d.PublishedAt
	v.PublishedAt = &published
	return v
}

// Shown is d as a sign-in screen shows it.
func (d Document) Shown() DocumentView {
	v := DocumentView{Kind: d.Kind, Version: d.Version, URL: d.URL}
	if d.Title != nil {
		v.Title = *d.Title
	}
	return v
}

// AcceptanceView is one acceptance as the API shows it (TermsAcceptance in
// the contract).
type AcceptanceView struct {
	Kind       string    `json:"kind"`
	Version    string    `json:"version"`
	AcceptedAt time.Time `json:"accepted_at"`
	ClientID   string    `json:"client_id,omitempty"`
}

// PublishRequest is the body of PUT /api/v1/tenants/{id}/terms/{kind}.
type PublishRequest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Title   string `json:"title"`
}
