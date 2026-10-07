package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/pkg/client"
)

// newSocialStartTestApp serves the social start route for one client, with a
// Hydra that knows the login flow "flow-1" and records rejections.
func newSocialStartTestApp(t *testing.T, cl *client.Client) (*fiber.App, *[]string) {
	t.Helper()
	var rejected []string
	hydraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/requests/login"):
			if r.URL.Query().Get("challenge") != "flow-1" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not_found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"challenge":"flow-1","client":{"client_id":"` + cl.ClientID + `"}}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/login/reject"):
			rejected = append(rejected, r.URL.Query().Get("challenge"))
			_, _ = w.Write([]byte(`{"redirect_to":"https://app.example.com/callback?error=invalid_request"}`))
		default:
			t.Errorf("unexpected Hydra call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(hydraSrv.Close)

	clients := newFakeClientService(cl)
	google := social.NewGoogleService(&config.GoogleOAuthConfig{ClientID: "google-app", RedirectURL: "https://api.example.com/auth/google/callback"},
		nil, nil, clients, zap.NewNop())
	h := NewSocialHandlerWithAllProviders(google, nil, nil, nil, nil, clients, hydra.NewClient(hydraSrv.URL), zap.NewNop(), nil,
		NewOAuthStateStore(newTestRedisClient(t)), testFrontendURL)
	app := fiber.New()
	app.Get("/login-flows/:flow/social/:provider", h.StartSocialLogin)
	return app, &rejected
}

func TestStartSocialLogin_SendsTheBrowserToTheProvider(t *testing.T) {
	app, rejected := newSocialStartTestApp(t, &client.Client{ID: uuid.New(), ClientID: "app", EnabledAuthProviders: []string{"google"}})

	resp := get(t, app, "/login-flows/flow-1/social/google", "")

	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != fiber.StatusFound || loc.Host != "accounts.google.com" {
		t.Fatalf("status %d location %q, want 302 to Google", resp.StatusCode, resp.Header.Get("Location"))
	}
	var stateCookie *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "oauth_state" {
			stateCookie = ck
		}
	}
	if stateCookie == nil || stateCookie.Value != loc.Query().Get("state") {
		t.Fatalf("oauth_state cookie %v does not bind the state sent to the provider (%q)", stateCookie, loc.Query().Get("state"))
	}
	if len(*rejected) != 0 {
		t.Errorf("rejected %v, want none", *rejected)
	}
}

// The screen only shows enabled providers; the server enforces the same rule.
func TestStartSocialLogin_ProviderNotOfferedByTheClient(t *testing.T) {
	app, rejected := newSocialStartTestApp(t, &client.Client{ID: uuid.New(), ClientID: "app", EnabledAuthProviders: []string{"email"}})

	resp := get(t, app, "/login-flows/flow-1/social/google", "")

	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "https://app.example.com/callback?error=invalid_request" {
		t.Fatalf("status %d location %q, want 302 to the rejection redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
	if len(*rejected) != 1 || (*rejected)[0] != "flow-1" {
		t.Errorf("rejected %v, want the flow", *rejected)
	}
}

func TestStartSocialLogin_UnknownFlowShowsTheErrorScreen(t *testing.T) {
	app, rejected := newSocialStartTestApp(t, &client.Client{ID: uuid.New(), ClientID: "app"})

	resp := get(t, app, "/login-flows/no-such-flow/social/google", "")

	loc := resp.Header.Get("Location")
	if resp.StatusCode != fiber.StatusFound || !strings.HasPrefix(loc, testFrontendURL+"/error?error=invalid_request") {
		t.Fatalf("status %d location %q, want 302 to the login UI's error screen", resp.StatusCode, loc)
	}
	if len(*rejected) != 0 {
		t.Errorf("rejected %v, want none — there is no flow to reject", *rejected)
	}
}
