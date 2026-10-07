package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
)

// fakeHydraLogout serves the Hydra admin calls a logout makes and records the
// session revocations.
type fakeHydraLogout struct {
	mu             sync.Mutex
	logoutClientID string
	revokeStatus   int
	acceptedURI    string
	revokedLogin   string
	revokedConsent string
	consentQuery   string
}

func (f *fakeHydraLogout) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/oauth2/auth/requests/logout":
			client := `null`
			if f.logoutClientID != "" {
				client = `{"client_id":"` + f.logoutClientID + `"}`
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"challenge":"chal-1","subject":"user-123","client":` + client + `}`))
		case r.Method == http.MethodPut && r.URL.Path == "/admin/oauth2/auth/requests/logout/accept":
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			f.acceptedURI = string(body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"redirect_to":"https://example.com/logged-out"}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/sessions/login"):
			f.revokedLogin = r.URL.Query().Get("subject")
			w.WriteHeader(f.revokeStatus)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/sessions/consent"):
			f.revokedConsent = r.URL.Query().Get("subject")
			f.consentQuery = r.URL.RawQuery
			w.WriteHeader(f.revokeStatus)
		default:
			t.Errorf("unexpected Hydra call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func runLogout(t *testing.T, f *fakeHydraLogout, clients client.Service, query string) *http.Response {
	t.Helper()
	srv := f.server(t)
	t.Cleanup(srv.Close)
	h := NewLogoutFlowHandler(clients, hydra.NewClient(srv.URL), zap.NewNop())
	app := fiber.New()
	app.Get("/logout", h.HandleLogout)
	resp, err := app.Test(httptest.NewRequest("GET", "/logout?"+query, nil))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// Accepting a logout only ends the browser session; tokens issued before it
// must also stop working, so every session of the subject is revoked.
func TestLogoutFlow_RevokesSessionsAfterAccept(t *testing.T) {
	f := &fakeHydraLogout{revokeStatus: http.StatusNoContent}
	resp := runLogout(t, f, newFakeClientService(), "logout_challenge=chal-1")

	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "https://example.com/logged-out" {
		t.Fatalf("status %d location %q, want 302 to Hydra's redirect_to", resp.StatusCode, resp.Header.Get("Location"))
	}
	if f.revokedLogin != "user-123" || f.revokedConsent != "user-123" {
		t.Errorf("revoked login=%q consent=%q, want user-123 for both", f.revokedLogin, f.revokedConsent)
	}
	if !strings.Contains(f.consentQuery, "all=true") {
		t.Errorf("consent revoke query %q, want all=true (every client)", f.consentQuery)
	}
}

// Revocation is best effort on this browser path: the user still gets sent
// back to their application when Hydra's revoke calls fail.
func TestLogoutFlow_RevocationFailureStillRedirects(t *testing.T) {
	f := &fakeHydraLogout{revokeStatus: http.StatusInternalServerError}
	resp := runLogout(t, f, newFakeClientService(), "logout_challenge=chal-1")
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302 despite revocation failure", resp.StatusCode)
	}
}

// A redirect the client's strict policy does not allow is refused with a
// fallback the logout screen can still send the user to.
func TestLogoutFlow_StrictPolicyRefusesUnlistedRedirect(t *testing.T) {
	f := &fakeHydraLogout{logoutClientID: "app", revokeStatus: http.StatusNoContent}
	clients := newFakeClientService(&client.Client{
		ClientID:               "app",
		LogoutRedirectPolicy:   "strict",
		PostLogoutRedirectURIs: []string{"https://app.example.com/bye"},
	})
	resp := runLogout(t, f, clients, "logout_challenge=chal-1&post_logout_redirect_uri=https%3A%2F%2Fevil.example%2F")
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if f.acceptedURI != "" {
		t.Errorf("logout was accepted (%s) despite the policy refusing the redirect", f.acceptedURI)
	}

	// The whitelisted redirect goes through and is handed to Hydra.
	f2 := &fakeHydraLogout{logoutClientID: "app", revokeStatus: http.StatusNoContent}
	resp = runLogout(t, f2, clients, "logout_challenge=chal-1&post_logout_redirect_uri=https%3A%2F%2Fapp.example.com%2Fbye")
	if resp.StatusCode != fiber.StatusFound || !strings.Contains(f2.acceptedURI, "https://app.example.com/bye") {
		t.Fatalf("status %d accepted %q, want 302 with the whitelisted URI", resp.StatusCode, f2.acceptedURI)
	}
}

func TestLogoutFlow_MissingChallengeIsBadRequest(t *testing.T) {
	h := NewLogoutFlowHandler(newFakeClientService(), hydra.NewClient("http://unused.invalid"), zap.NewNop())
	app := fiber.New()
	app.Get("/logout", h.HandleLogout)
	resp, _ := app.Test(httptest.NewRequest("GET", "/logout", nil))
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
