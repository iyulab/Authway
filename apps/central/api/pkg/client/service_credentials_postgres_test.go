package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"authway/apps/central/api/internal/hydra"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// capturingHydra records the client each create call sends to the
// authorization server, so a test can check what was registered there and not
// only what Authway stored.
type capturingHydra struct {
	mu      sync.Mutex
	created []hydra.OAuth2Client
}

func (h *capturingHydra) client(t *testing.T) *hydra.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPost {
			var c hydra.OAuth2Client
			_ = json.Unmarshal(body, &c)
			h.mu.Lock()
			h.created = append(h.created, c)
			h.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL)
}

func (h *capturingHydra) last(t *testing.T) hydra.OAuth2Client {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.created) == 0 {
		t.Fatal("nothing was registered with the authorization server")
	}
	return h.created[len(h.created)-1]
}

// Which credentials a new client ends up with, and what the authorization
// server is told: public clients never hold a secret and authenticate with
// "none"; confidential clients get both halves — supplied together or
// generated together, never one of each.
func TestCreate_CredentialRules(t *testing.T) {
	db := setupPostgres(t)
	tenantID := seedTenant(t, db)
	suffix := uuid.NewString()[:8]

	cases := []struct {
		name         string
		public       bool
		clientID     string
		clientSecret string
		grants       []string
		wantID       string // "" = generated
		wantSecret   string // "" with public=false means generated
		wantMethod   string
		wantCode     string // ConfigError code when refused
	}{
		{name: "public with its own id", public: true, clientID: "spa_" + suffix,
			grants: []string{"authorization_code"}, wantID: "spa_" + suffix, wantMethod: "none"},
		{name: "public with a generated id", public: true,
			grants: []string{"authorization_code"}, wantMethod: "none"},
		{name: "confidential with both halves", clientID: "backend_" + suffix, clientSecret: "supplied-secret-" + suffix,
			grants: []string{"client_credentials"}, wantID: "backend_" + suffix, wantSecret: "supplied-secret-" + suffix, wantMethod: "client_secret_post"},
		{name: "confidential with neither", grants: []string{"client_credentials"}, wantMethod: "client_secret_post"},
		{name: "confidential with only an id", clientID: "half_" + suffix,
			grants: []string{"client_credentials"}, wantCode: "confidential_client_partial_credentials"},
		{name: "confidential with only a secret", clientSecret: "lonely-secret",
			grants: []string{"client_credentials"}, wantCode: "confidential_client_partial_credentials"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &capturingHydra{}
			svc := NewService(db, zap.NewNop(), h.client(t))
			req := &CreateClientRequest{
				TenantID: tenantID, Name: "cred-rules-" + suffix,
				Public: tc.public, ClientID: tc.clientID, ClientSecret: tc.clientSecret,
				GrantTypes: tc.grants, Scopes: []string{"openid"},
			}
			if tc.public {
				req.RedirectURIs = []string{"https://example.com/callback"}
				req.AllowedOrigins = []string{"https://example.com"}
			}

			created, creds, err := svc.Create(req)
			if tc.wantCode != "" {
				cerr, ok := err.(*ConfigError)
				if !ok || cerr.Code != tc.wantCode {
					t.Fatalf("Create = %v, want ConfigError %s", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })

			if tc.wantID != "" && created.ClientID != tc.wantID {
				t.Errorf("client_id = %q, want %q", created.ClientID, tc.wantID)
			}
			if tc.wantID == "" && !strings.HasPrefix(created.ClientID, "authway_") {
				t.Errorf("generated client_id = %q, want the authway_ prefix", created.ClientID)
			}
			switch {
			case tc.public && (created.ClientSecret != "" || creds.ClientSecret != ""):
				t.Error("a public client was given a secret")
			case !tc.public && tc.wantSecret != "" && creds.ClientSecret != tc.wantSecret:
				t.Errorf("secret = %q, want the supplied one", creds.ClientSecret)
			case !tc.public && tc.wantSecret == "" && len(creds.ClientSecret) < 32:
				t.Errorf("generated secret %q is too short", creds.ClientSecret)
			}

			sent := h.last(t)
			if sent.ClientID != created.ClientID || sent.ClientSecret != creds.ClientSecret || sent.TokenEndpointAuthMethod != tc.wantMethod {
				t.Errorf("registered %q/%q/%q, want %q/%q/%q", sent.ClientID, sent.ClientSecret, sent.TokenEndpointAuthMethod,
					created.ClientID, creds.ClientSecret, tc.wantMethod)
			}

			// The stored pair is what authenticates the client later.
			if _, err := svc.ValidateClient(created.ClientID, creds.ClientSecret); err != nil {
				t.Errorf("ValidateClient with the issued credentials: %v", err)
			}
			if !tc.public {
				if _, err := svc.ValidateClient(created.ClientID, "wrong-secret"); err == nil {
					t.Error("ValidateClient accepted a wrong secret")
				}
			}
		})
	}
}
