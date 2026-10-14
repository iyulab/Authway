package terms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors the same-named helper in pkg/claims: the documents
// and acceptances live in Postgres (a partial unique index keeps one current
// version per kind).
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres terms tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func fixture(t *testing.T, db *gorm.DB) (tenantID, userID uuid.UUID) {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := tenant.NewService(db).CreateTenant(tenant.CreateTenantRequest{Name: "terms-test-" + suffix, Slug: "terms-test-" + suffix})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	u, err := user.NewService(db, zap.NewNop()).Create(tn.ID, &user.CreateUserRequest{
		Email: fmt.Sprintf("terms-%s@example.com", suffix), Name: "Terms Test User",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
		db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID)
	})
	return tn.ID, u.ID
}

// clock advances a second per call, so versions published in one test have
// distinct times.
func clock() func() time.Time {
	t := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	return func() time.Time { t = t.Add(time.Second); return t }
}

func newService(db *gorm.DB) *service { return &service{db: db, now: clock()} }

func publish(t *testing.T, s Service, tenantID uuid.UUID, kind, version string) (Document, bool) {
	t.Helper()
	d, created, err := s.Publish(context.Background(), tenantID, kind, PublishRequest{Version: version, URL: "https://example.com/" + kind + "/" + version})
	if err != nil {
		t.Fatalf("publish %s %s: %v", kind, version, err)
	}
	return d, created
}

func kinds(docs []Document) []string {
	out := []string{}
	for _, d := range docs {
		out = append(out, d.Kind+"@"+d.Version)
	}
	return out
}

func TestTerms_NothingPublishedOwesNothing(t *testing.T) {
	db := setupPostgres(t)
	s := newService(db)
	tenantID, userID := fixture(t, db)
	due, err := s.Owed(context.Background(), tenantID, userID)
	if err != nil || len(due) != 0 {
		t.Fatalf("Owed = %v, %v; want nothing", kinds(due), err)
	}
	// Accepting nothing when nothing is owed is not a mismatch.
	if err := s.Accept(context.Background(), tenantID, userID, nil, "c"); err != nil {
		t.Fatalf("Accept(nothing) = %v", err)
	}
}

func TestTerms_PublishNewVersionReplacesCurrent(t *testing.T) {
	db := setupPostgres(t)
	s := newService(db)
	ctx := context.Background()
	tenantID, userID := fixture(t, db)

	if _, created := publish(t, s, tenantID, "terms_of_service", "1"); !created {
		t.Fatal("first version: created = false")
	}
	publish(t, s, tenantID, "privacy_policy", "a")
	due, _ := s.Owed(ctx, tenantID, userID)
	if got := kinds(due); fmt.Sprint(got) != "[privacy_policy@a terms_of_service@1]" {
		t.Fatalf("Owed = %v", got)
	}
	if err := s.Accept(ctx, tenantID, userID, []KindVersion{{"terms_of_service", "1"}, {"privacy_policy", "a"}}, "app-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if due, _ := s.Owed(ctx, tenantID, userID); len(due) != 0 {
		t.Fatalf("after accepting, Owed = %v", kinds(due))
	}

	// Same version again: only the address changes, nobody owes it again.
	d, created, err := s.Publish(ctx, tenantID, "terms_of_service", PublishRequest{Version: "1", URL: "https://example.com/moved", Title: "Terms"})
	if err != nil || created || d.URL != "https://example.com/moved" || d.Title == nil || *d.Title != "Terms" {
		t.Fatalf("republish same version = %+v, created %v, %v", d, created, err)
	}
	if due, _ := s.Owed(ctx, tenantID, userID); len(due) != 0 {
		t.Fatalf("same version made the user owe it again: %v", kinds(due))
	}

	// A new version: owed again, and only that kind.
	if _, created := publish(t, s, tenantID, "terms_of_service", "2"); !created {
		t.Fatal("new version: created = false")
	}
	due, _ = s.Owed(ctx, tenantID, userID)
	if got := kinds(due); fmt.Sprint(got) != "[terms_of_service@2]" {
		t.Fatalf("after a new version, Owed = %v", got)
	}
	cur, _ := s.Current(ctx, tenantID)
	if got := kinds(cur); fmt.Sprint(got) != "[privacy_policy@a terms_of_service@2]" {
		t.Fatalf("Current = %v", got)
	}

	// Back to version 1: the user accepted it, so nothing is owed.
	if _, created := publish(t, s, tenantID, "terms_of_service", "1"); !created {
		t.Fatal("restoring an earlier version: created = false")
	}
	if due, _ := s.Owed(ctx, tenantID, userID); len(due) != 0 {
		t.Fatalf("restored version already accepted, but Owed = %v", kinds(due))
	}
	var versions int64
	db.Model(&Document{}).Where("tenant_id = ? AND kind = ?", tenantID, "terms_of_service").Count(&versions)
	if versions != 2 {
		t.Fatalf("versions kept = %d, want 2", versions)
	}
}

func TestTerms_AcceptMustNameExactlyWhatIsOwed(t *testing.T) {
	db := setupPostgres(t)
	s := newService(db)
	ctx := context.Background()
	tenantID, userID := fixture(t, db)
	publish(t, s, tenantID, "terms_of_service", "1")
	publish(t, s, tenantID, "privacy_policy", "a")

	for name, accepted := range map[string][]KindVersion{
		"one missing":   {{"terms_of_service", "1"}},
		"old version":   {{"terms_of_service", "0"}, {"privacy_policy", "a"}},
		"twice the one": {{"terms_of_service", "1"}, {"terms_of_service", "1"}},
		"one extra":     {{"terms_of_service", "1"}, {"privacy_policy", "a"}, {"other", "x"}},
	} {
		err := s.Accept(ctx, tenantID, userID, accepted, "app")
		var mismatch *MismatchError
		if !errors.As(err, &mismatch) || len(mismatch.Owed) != 2 {
			t.Fatalf("%s: Accept = %v, want a mismatch carrying both documents", name, err)
		}
	}
	var recorded int64
	db.Model(&Acceptance{}).Where("user_id = ?", userID).Count(&recorded)
	if recorded != 0 {
		t.Fatalf("a refused acceptance recorded %d rows", recorded)
	}
}

func TestTerms_RetireStopsAskingAndKeepsTheRecord(t *testing.T) {
	db := setupPostgres(t)
	s := newService(db)
	ctx := context.Background()
	tenantID, userID := fixture(t, db)
	publish(t, s, tenantID, "terms_of_service", "1")
	if err := s.Accept(ctx, tenantID, userID, []KindVersion{{"terms_of_service", "1"}}, "app-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	publish(t, s, tenantID, "privacy_policy", "a")

	if err := s.Retire(ctx, tenantID, "privacy_policy"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if due, _ := s.Owed(ctx, tenantID, userID); len(due) != 0 {
		t.Fatalf("retired document still owed: %v", kinds(due))
	}
	if err := s.Retire(ctx, tenantID, "privacy_policy"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retiring twice = %v, want ErrNotFound", err)
	}

	list, err := s.Acceptances(ctx, userID)
	if err != nil || len(list) != 1 || list[0].Kind != "terms_of_service" || list[0].Version != "1" || list[0].ClientID != "app-1" {
		t.Fatalf("Acceptances = %+v, %v", list, err)
	}
}

func TestTerms_OtherTenantsDocumentsAreNotOwed(t *testing.T) {
	db := setupPostgres(t)
	s := newService(db)
	tenantID, userID := fixture(t, db)
	otherTenant, _ := fixture(t, db)
	publish(t, s, otherTenant, "terms_of_service", "1")
	if due, _ := s.Owed(context.Background(), tenantID, userID); len(due) != 0 {
		t.Fatalf("another tenant's document is owed: %v", kinds(due))
	}
}

func TestTerms_PublishRefusesWhatIsNotADocument(t *testing.T) {
	db := setupPostgres(t)
	s := newService(db)
	tenantID, _ := fixture(t, db)
	for name, c := range map[string]struct {
		kind string
		req  PublishRequest
	}{
		"kind with a capital":  {"Terms", PublishRequest{Version: "1", URL: "https://example.com/t"}},
		"kind too long":        {"k0123456789012345678901234567890123456789", PublishRequest{Version: "1", URL: "https://example.com/t"}},
		"no version":           {"terms", PublishRequest{Version: "  ", URL: "https://example.com/t"}},
		"plain http":           {"terms", PublishRequest{Version: "1", URL: "http://example.com/t"}},
		"not an address":       {"terms", PublishRequest{Version: "1", URL: "terms.html"}},
		"version with newline": {"terms", PublishRequest{Version: "1\n2", URL: "https://example.com/t"}},
	} {
		_, _, err := s.Publish(context.Background(), tenantID, c.kind, c.req)
		var invalid *InvalidError
		if !errors.As(err, &invalid) {
			t.Fatalf("%s: Publish = %v, want InvalidError", name, err)
		}
	}
}
