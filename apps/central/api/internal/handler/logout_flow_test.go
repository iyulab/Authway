package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

// fakeHydraLogout serves the Hydra admin calls a logout makes and records the
// session revocations.
type fakeHydraLogout struct {
	mu             sync.Mutex
	revokeStatus   int
	acceptBody     string
	acceptCalls    int
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
			if r.URL.Query().Get("logout_challenge") != "chal-1=" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not_found"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"challenge":"chal-1=","subject":"user-123","client":{"client_id":"app"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/admin/oauth2/auth/requests/logout/accept":
			f.acceptCalls++
			if r.Body != nil {
				var b strings.Builder
				buf := make([]byte, 512)
				for {
					n, err := r.Body.Read(buf)
					b.Write(buf[:n])
					if err != nil {
						break
					}
				}
				f.acceptBody = b.String()
			}
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

func runLogout(t *testing.T, f *fakeHydraLogout, flow string) (int, map[string]any) {
	t.Helper()
	srv := f.server(t)
	t.Cleanup(srv.Close)
	h := NewLogoutFlowHandler(hydra.NewClient(srv.URL), zap.NewNop())
	app := fiber.New()
	app.Post("/logout-flows/:flow", h.CompleteLogout)
	resp, err := app.Test(httptest.NewRequest("POST", "/logout-flows/"+flow, nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

// Accepting a logout only ends the browser session; tokens issued before it
// must also stop working, so every session of the subject is revoked. Where
// the browser goes next is Hydra's answer, passed through unchanged.
func TestLogoutFlow_RevokesSessionsAfterAccept(t *testing.T) {
	f := &fakeHydraLogout{revokeStatus: http.StatusNoContent}
	status, body := runLogout(t, f, "chal-1%3D")

	if status != fiber.StatusOK || body["next"] != "redirect" || body["redirect_to"] != "https://example.com/logged-out" {
		t.Fatalf("status %d body %v, want next=redirect to Hydra's redirect_to", status, body)
	}
	if f.revokedLogin != "user-123" || f.revokedConsent != "user-123" {
		t.Errorf("revoked login=%q consent=%q, want user-123 for both", f.revokedLogin, f.revokedConsent)
	}
	if !strings.Contains(f.consentQuery, "all=true") {
		t.Errorf("consent revoke query %q, want all=true (every client)", f.consentQuery)
	}
	if f.acceptBody != "" {
		t.Errorf("accept sent body %q; Hydra takes none and decides the destination itself", f.acceptBody)
	}
}

// Revocation is best effort on this browser path: the user still gets sent
// back to their application when Hydra's revoke calls fail.
func TestLogoutFlow_RevocationFailureStillRedirects(t *testing.T) {
	f := &fakeHydraLogout{revokeStatus: http.StatusInternalServerError}
	status, body := runLogout(t, f, "chal-1%3D")
	if status != fiber.StatusOK || body["redirect_to"] != "https://example.com/logged-out" {
		t.Fatalf("status %d body %v, want the redirect despite revocation failure", status, body)
	}
}

func TestLogoutFlow_UnknownFlowIsAClientError(t *testing.T) {
	f := &fakeHydraLogout{revokeStatus: http.StatusNoContent}
	status, body := runLogout(t, f, "no-such-flow")
	if status != fiber.StatusBadRequest || body["code"] != "invalid_flow" {
		t.Fatalf("status %d body %v, want 400 invalid_flow", status, body)
	}
	if f.acceptCalls != 0 {
		t.Errorf("accept called %d times for an unknown flow", f.acceptCalls)
	}
}
