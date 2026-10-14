package terms

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when a tenant has no current document of a kind.
var ErrNotFound = errors.New("no current document of that kind")

// InvalidError is a refused request: what is wrong with it, for the caller.
type InvalidError struct{ Reason string }

func (e *InvalidError) Error() string { return e.Reason }

// MismatchError refuses an acceptance that does not name exactly the
// documents owed — one was revised, added or retired after the user was
// shown them. Owed is what to show instead.
type MismatchError struct{ Owed []Document }

func (e *MismatchError) Error() string {
	return "the documents accepted are not the ones owed now"
}

// KindVersion names a document version a user accepts.
type KindVersion struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

// Service keeps tenants' documents and users' acceptances.
type Service interface {
	// Current returns the tenant's current documents, ordered by kind.
	Current(ctx context.Context, tenantID uuid.UUID) ([]Document, error)
	// Publish makes req's version the current document of kind. created is
	// false when that version was current already: only its address and
	// title change, and nobody owes it again.
	Publish(ctx context.Context, tenantID uuid.UUID, kind string, req PublishRequest) (doc Document, created bool, err error)
	// Retire stops asking for kind. ErrNotFound when it is not asked for.
	Retire(ctx context.Context, tenantID uuid.UUID, kind string) error
	// Owed returns the tenant's current documents the user has not accepted,
	// ordered by kind. None means the user owes nothing.
	Owed(ctx context.Context, tenantID, userID uuid.UUID) ([]Document, error)
	// Accept records that the user accepted exactly the documents owed now,
	// signing in to clientID. Any other list is a *MismatchError and records
	// nothing.
	Accept(ctx context.Context, tenantID, userID uuid.UUID, accepted []KindVersion, clientID string) error
	// Acceptances returns what the user accepted, most recent first.
	Acceptances(ctx context.Context, userID uuid.UUID) ([]AcceptanceView, error)
}

type service struct {
	db  *gorm.DB
	now func() time.Time
}

// NewService returns a Service storing in db.
func NewService(db *gorm.DB) Service {
	return &service{db: db, now: time.Now}
}

var kindPattern = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// CheckKind refuses a kind that is not a document name.
func CheckKind(kind string) error {
	if !kindPattern.MatchString(kind) {
		return &InvalidError{Reason: "kind must be 1 to 40 lowercase letters, digits or underscores"}
	}
	return nil
}

func checkPublish(req *PublishRequest) error {
	req.Version = strings.TrimSpace(req.Version)
	req.URL = strings.TrimSpace(req.URL)
	req.Title = strings.TrimSpace(req.Title)
	if req.Version == "" || len(req.Version) > 40 || strings.IndexFunc(req.Version, unicode.IsControl) >= 0 {
		return &InvalidError{Reason: "version must be 1 to 40 characters"}
	}
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return &InvalidError{Reason: "url must be an https address"}
	}
	if len(req.Title) > 200 || strings.IndexFunc(req.Title, unicode.IsControl) >= 0 {
		return &InvalidError{Reason: "title must be at most 200 characters"}
	}
	return nil
}

func (s *service) Current(ctx context.Context, tenantID uuid.UUID) ([]Document, error) {
	return current(s.db.WithContext(ctx), tenantID)
}

func current(db *gorm.DB, tenantID uuid.UUID) ([]Document, error) {
	var docs []Document
	if err := db.Where("tenant_id = ? AND retired_at IS NULL", tenantID).Order("kind").Find(&docs).Error; err != nil {
		return nil, fmt.Errorf("failed to read the tenant's documents: %w", err)
	}
	return docs, nil
}

func (s *service) Publish(ctx context.Context, tenantID uuid.UUID, kind string, req PublishRequest) (Document, bool, error) {
	if err := CheckKind(kind); err != nil {
		return Document{}, false, err
	}
	if err := checkPublish(&req); err != nil {
		return Document{}, false, err
	}
	var title *string
	if req.Title != "" {
		title = &req.Title
	}
	var doc Document
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The kind's versions, locked: two publishes of one kind must not
		// both see "no current version" and both make one.
		var versions []Document
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND kind = ?", tenantID, kind).Find(&versions).Error; err != nil {
			return fmt.Errorf("failed to read the document's versions: %w", err)
		}
		now := s.now()
		var cur, same *Document
		for i := range versions {
			if versions[i].RetiredAt == nil {
				cur = &versions[i]
			}
			if versions[i].Version == req.Version {
				same = &versions[i]
			}
		}
		if cur != nil && same != nil && cur.ID == same.ID {
			// Already current: a new address or title is not a new document.
			doc = *cur
			doc.URL, doc.Title = req.URL, title
			return tx.Model(&Document{}).Where("id = ?", doc.ID).
				Updates(map[string]any{"url": doc.URL, "title": doc.Title}).Error
		}
		created = true
		if cur != nil {
			if err := tx.Model(&Document{}).Where("id = ?", cur.ID).Update("retired_at", now).Error; err != nil {
				return fmt.Errorf("failed to retire the current version: %w", err)
			}
		}
		if same != nil {
			// An earlier version made current again is the same document:
			// whoever accepted it then has accepted it.
			doc = *same
			doc.URL, doc.Title, doc.PublishedAt, doc.RetiredAt = req.URL, title, now, nil
			return tx.Model(&Document{}).Where("id = ?", doc.ID).Updates(map[string]any{
				"url": doc.URL, "title": doc.Title, "published_at": now, "retired_at": nil,
			}).Error
		}
		doc = Document{
			ID: uuid.New(), TenantID: tenantID, Kind: kind, Version: req.Version,
			URL: req.URL, Title: title, PublishedAt: now,
		}
		return tx.Create(&doc).Error
	})
	if err != nil {
		return Document{}, false, err
	}
	return doc, created, nil
}

func (s *service) Retire(ctx context.Context, tenantID uuid.UUID, kind string) error {
	if err := CheckKind(kind); err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Model(&Document{}).
		Where("tenant_id = ? AND kind = ? AND retired_at IS NULL", tenantID, kind).
		Update("retired_at", s.now())
	if res.Error != nil {
		return fmt.Errorf("failed to retire the document: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *service) Owed(ctx context.Context, tenantID, userID uuid.UUID) ([]Document, error) {
	return owed(s.db.WithContext(ctx), tenantID, userID)
}

func owed(db *gorm.DB, tenantID, userID uuid.UUID) ([]Document, error) {
	var docs []Document
	err := db.Where("tenant_id = ? AND retired_at IS NULL", tenantID).
		Where("NOT EXISTS (SELECT 1 FROM user_terms_acceptances a WHERE a.terms_id = tenant_terms.id AND a.user_id = ?)", userID).
		Order("kind").Find(&docs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to read the documents owed: %w", err)
	}
	return docs, nil
}

func (s *service) Accept(ctx context.Context, tenantID, userID uuid.UUID, accepted []KindVersion, clientID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		due, err := owed(tx, tenantID, userID)
		if err != nil {
			return err
		}
		if !sameDocuments(due, accepted) {
			return &MismatchError{Owed: due}
		}
		var client *string
		if clientID != "" {
			client = &clientID
		}
		now := s.now()
		for _, d := range due {
			row := Acceptance{ID: uuid.New(), UserID: userID, TermsID: d.ID, ClientID: client, AcceptedAt: now}
			// Accepting twice at once records it once.
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return fmt.Errorf("failed to record the acceptance: %w", err)
			}
		}
		return nil
	})
}

// sameDocuments reports whether accepted names exactly the documents due,
// each once.
func sameDocuments(due []Document, accepted []KindVersion) bool {
	if len(due) != len(accepted) {
		return false
	}
	want := make([]string, 0, len(due))
	for _, d := range due {
		want = append(want, d.Kind+"\x00"+d.Version)
	}
	got := make([]string, 0, len(accepted))
	for _, a := range accepted {
		got = append(got, a.Kind+"\x00"+a.Version)
	}
	slices.Sort(want)
	slices.Sort(got)
	return slices.Equal(want, got)
}

func (s *service) Acceptances(ctx context.Context, userID uuid.UUID) ([]AcceptanceView, error) {
	var rows []struct {
		Kind       string
		Version    string
		AcceptedAt time.Time
		ClientID   *string
	}
	err := s.db.WithContext(ctx).Table("user_terms_acceptances AS a").
		Select("t.kind, t.version, a.accepted_at, a.client_id").
		Joins("JOIN tenant_terms t ON t.id = a.terms_id").
		Where("a.user_id = ?", userID).
		Order("a.accepted_at DESC, t.kind").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to read the acceptances: %w", err)
	}
	out := make([]AcceptanceView, 0, len(rows))
	for _, r := range rows {
		v := AcceptanceView{Kind: r.Kind, Version: r.Version, AcceptedAt: r.AcceptedAt}
		if r.ClientID != nil {
			v.ClientID = *r.ClientID
		}
		out = append(out, v)
	}
	return out, nil
}
