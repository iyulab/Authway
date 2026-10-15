package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

// A client's logo was kept and returned, and no screen showed it. It now
// reaches the authorization server as the client's logo_uri, which the
// sign-in and consent screens read. An update that does not name it — or the
// description — leaves it alone.
func TestLogo_ReachesTheAuthorizationServerAndChangesOnlyWhenNamed(t *testing.T) {
	db := setupPostgres(t)
	var mu sync.Mutex
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			LogoURI *string `json:"logo_uri"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		if body.LogoURI != nil {
			sent = append(sent, *body.LogoURI)
		} else {
			sent = append(sent, "")
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write([]byte(`{"client_id":"stub"}`))
	}))
	t.Cleanup(srv.Close)
	svc := NewService(db, zap.NewNop(), hydra.NewClient(srv.URL))
	last := func() string { mu.Lock(); defer mu.Unlock(); return sent[len(sent)-1] }

	logo := "https://app.example.com/logo.png"
	created, _, err := svc.Create(&CreateClientRequest{
		TenantID:     seedTenant(t, db),
		Name:         "logo-" + uuid.NewString()[:8],
		Description:  "kept",
		Logo:         logo,
		RedirectURIs: []string{"https://app.example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })
	if got := last(); got != logo {
		t.Errorf("the authorization server got logo_uri %q, want %q", got, logo)
	}

	renamed := "logo-renamed-" + uuid.NewString()[:8]
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{Name: renamed}); err != nil {
		t.Fatalf("Update (name only): %v", err)
	}
	stored, _ := svc.GetByID(created.ID)
	if stored.Logo != logo || stored.Description != "kept" {
		t.Errorf("an update naming neither changed them: logo %q, description %q", stored.Logo, stored.Description)
	}
	if got := last(); got != logo {
		t.Errorf("after an update the authorization server has logo_uri %q, want it kept", got)
	}

	for _, bad := range []string{"http://app.example.com/logo.png", "not a url", "https://app.example.com/logo.png#x"} {
		b := bad
		if _, _, err := svc.Update(created.ID, &UpdateClientRequest{Logo: &b}); err == nil {
			t.Errorf("logo %q was accepted on update", bad)
		}
	}
	if _, _, err := svc.Create(&CreateClientRequest{
		TenantID: seedTenant(t, db), Name: "logo-bad-" + uuid.NewString()[:8], Logo: "http://app.example.com/logo.png",
		RedirectURIs: []string{"https://app.example.com/callback"}, GrantTypes: []string{"authorization_code"}, Scopes: []string{"openid"},
	}); err == nil {
		t.Error("an http logo was accepted on create")
	}

	empty := ""
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{Logo: &empty, Description: &empty}); err != nil {
		t.Fatalf("Update (clear): %v", err)
	}
	if stored, _ = svc.GetByID(created.ID); stored.Logo != "" || stored.Description != "" {
		t.Errorf("after clearing: logo %q, description %q", stored.Logo, stored.Description)
	}
	if got := last(); got != "" {
		t.Errorf("after clearing the authorization server has logo_uri %q", got)
	}
}
