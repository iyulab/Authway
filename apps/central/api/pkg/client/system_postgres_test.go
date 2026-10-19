package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"authway/apps/central/api/internal/hydra"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// freshTenant inserts a tenant of its own, so its system clients are this
// test's alone.
func freshTenant(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	slug := "system-clients-" + id.String()[:8]
	if err := db.Exec(`INSERT INTO tenants (id, name, slug, description, active) VALUES (?, ?, ?, '', true)`, id, slug, slug).Error; err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM clients WHERE tenant_id = ?`, id)
		db.Exec(`DELETE FROM tenants WHERE id = ?`, id)
	})
	return id
}

// registrationRecorder stands in for the authorization server's admin API and keeps
// the registrations it was sent for a changed client.
type registrationRecorder struct {
	mu      sync.Mutex
	updated []hydra.OAuth2Client
}

func (r *registrationRecorder) client(t *testing.T) *hydra.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if req.Method == http.MethodPut {
			var reg hydra.OAuth2Client
			_ = json.Unmarshal(body, &reg)
			r.mu.Lock()
			r.updated = append(r.updated, reg)
			r.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL)
}

// A tenant holds one system client of each kind: the system tenant has the
// account screen's and the console's side by side, and asking for one never
// answers with the other.
func TestSystemClients_OneOfEachKind(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	tenant := freshTenant(t, db)

	account, err := svc.EnsureAccountClient(tenant, "https://login.example.test")
	if err != nil {
		t.Fatalf("EnsureAccountClient: %v", err)
	}
	console, err := svc.EnsureConsoleClient(tenant, "https://console.example.test/")
	if err != nil {
		t.Fatalf("EnsureConsoleClient: %v", err)
	}
	if account.ID == console.ID {
		t.Fatal("the console's client is the account screen's")
	}
	if !console.System || console.SystemKind != SystemKindConsole || account.SystemKind != SystemKindAccount {
		t.Fatalf("kinds: account %q, console %q (system %v)", account.SystemKind, console.SystemKind, console.System)
	}
	if !slices.Equal([]string(console.RedirectURIs), []string{"https://console.example.test/callback"}) ||
		!slices.Equal([]string(console.Audience), []string{ConsoleAudience}) ||
		!slices.Equal([]string(console.GrantTypes), []string{"authorization_code"}) || !console.Public {
		t.Fatalf("console client registered as %+v", console)
	}

	again, err := svc.EnsureAccountClient(tenant, "https://login.example.test")
	if err != nil || again.ID != account.ID {
		t.Fatalf("EnsureAccountClient again = %v, %v; want the same client %v", again, err, account.ID)
	}
	againConsole, err := svc.EnsureConsoleClient(tenant, "https://console.example.test")
	if err != nil || againConsole.ID != console.ID {
		t.Fatalf("EnsureConsoleClient again = %v, %v; want the same client %v", againConsole, err, console.ID)
	}

	if _, _, err := svc.Update(console.ID, &UpdateClientRequest{Name: "taken over"}); err != ErrSystemClient {
		t.Fatalf("Update of the console's client = %v, want ErrSystemClient", err)
	}
}

// A system client follows the configured address: a console or login UI that
// moved sends its users back to where it is now, at the authorization server
// too.
func TestSystemClients_FollowAMovedAddress(t *testing.T) {
	db := setupPostgres(t)
	rec := &registrationRecorder{}
	svc := NewService(db, zap.NewNop(), rec.client(t))
	tenant := freshTenant(t, db)

	if _, err := svc.EnsureConsoleClient(tenant, "https://old.example.test/console"); err != nil {
		t.Fatalf("EnsureConsoleClient: %v", err)
	}
	moved, err := svc.EnsureConsoleClient(tenant, "https://new.example.test/console")
	if err != nil {
		t.Fatalf("EnsureConsoleClient after the move: %v", err)
	}
	want := []string{"https://new.example.test/console/callback"}
	if !slices.Equal([]string(moved.RedirectURIs), want) || !slices.Equal([]string(moved.AllowedOrigins), []string{"https://new.example.test"}) {
		t.Fatalf("after the move the client holds %v / %v", moved.RedirectURIs, moved.AllowedOrigins)
	}
	var stored Client
	if err := db.First(&stored, "id = ?", moved.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !slices.Equal([]string(stored.RedirectURIs), want) || !slices.Equal([]string(stored.PostLogoutRedirectURIs), []string{"https://new.example.test/console/"}) {
		t.Fatalf("stored %v / %v", stored.RedirectURIs, stored.PostLogoutRedirectURIs)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.updated) != 1 || !slices.Equal(rec.updated[0].RedirectUris, want) {
		t.Fatalf("the authorization server was sent %+v, want one registration redirecting to %v", rec.updated, want)
	}

	// Unchanged, nothing is sent again.
	if _, err := svc.EnsureConsoleClient(tenant, "https://new.example.test/console"); err != nil {
		t.Fatalf("EnsureConsoleClient unchanged: %v", err)
	}
	if len(rec.updated) != 1 {
		t.Fatalf("an unchanged address was registered anew: %d registrations", len(rec.updated))
	}
}
