package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/internal/hydra"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests run the real client service against a real Postgres, because the
// column constraints that matter here do not exist in the SQLite harness the
// other service tests use. `clients.redirect_uris` is `text[] NOT NULL`, and
// AutoMigrate-from-Go-struct never reproduces that — so a nil slice sails
// through SQLite and only explodes on a real deployment.
//
// Gated on the same DSN as the migration tests. The schema is brought up by the
// real migrator rather than assumed: RunMigrations is idempotent, so this both
// bootstraps a blank database and no-ops on one that is already current.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres client tests")
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

// stubHydra stands in for the Hydra admin API so these tests exercise the
// persistence path without needing a running Hydra.
func stubHydra(t *testing.T) *hydra.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write([]byte(`{"client_id":"stub"}`))
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL)
}

// seedTenant returns an existing tenant id as a string. Scanning straight into
// uuid.UUID fails here — it is a [16]byte, and database/sql hands the pgx text
// representation to it — so the id stays a string all the way to the request.
func seedTenant(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var id string
	if err := db.Raw(`SELECT id::text FROM tenants LIMIT 1`).Scan(&id).Error; err != nil {
		t.Fatalf("read tenant: %v", err)
	}
	if id == "" {
		t.Skip("no tenant row available in the target database")
	}
	return id
}

// TestCreateClient_MachineToMachine_PersistsEmptyRedirectURIs is the regression
// guard for the defect that shipped in the first cut of grant-conditional
// redirect_uris: validation correctly stopped requiring redirect URIs for
// client_credentials clients, but the model still handed GORM a nil slice.
// GORM writes an explicit NULL for that rather than omitting the column, so the
// column DEFAULT '{}' never applied and every M2M create failed with
// `null value in column "redirect_uris" violates not-null constraint` — the
// exact scenario the change was supposed to enable.
func TestCreateClient_MachineToMachine_PersistsEmptyRedirectURIs(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	tenantID := seedTenant(t, db)

	name := "regress-m2m-" + uuid.NewString()[:8]
	created, _, err := svc.Create(&CreateClientRequest{
		TenantID:   tenantID,
		Name:       name,
		Public:     false,
		GrantTypes: []string{"client_credentials"},
		Scopes:     []string{"api"},
		// RedirectURIs deliberately omitted — that is the whole point.
	})
	if err != nil {
		t.Fatalf("Create failed for a client_credentials-only client: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })

	if created.RedirectURIs == nil {
		t.Error("in-memory model has nil RedirectURIs; it must be an empty array")
	}

	// The stored row must be an empty array, not NULL — and no dummy redirect
	// may leak into the logout URIs either.
	var isNull bool
	if err := db.Raw(`SELECT redirect_uris IS NULL FROM clients WHERE id = ?`, created.ID).Scan(&isNull).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if isNull {
		t.Error("stored redirect_uris is NULL; expected an empty array")
	}

	var count int
	if err := db.Raw(
		`SELECT coalesce(array_length(post_logout_redirect_uris, 1), 0) FROM clients WHERE id = ?`,
		created.ID,
	).Scan(&count).Error; err != nil {
		t.Fatalf("read back logout URIs: %v", err)
	}
	if count != 0 {
		t.Errorf("post_logout_redirect_uris = %d entries; a client with no redirect URIs must not gain any", count)
	}
}
