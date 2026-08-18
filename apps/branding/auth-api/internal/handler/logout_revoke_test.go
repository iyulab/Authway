package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"authway/apps/branding/auth-api/internal/config"
	"authway/apps/branding/auth-api/internal/service"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// TestHandleLogout_RevokesSessionsAfterAccept is the regression this cycle
// exists for (ISSUE-Authway-20260818-oidc-logout-does-not-revoke-issued-
// tokens, HD-12): accepting a Hydra RP-initiated logout only ends the
// browser's login session — it does not by itself invalidate previously
// issued access/refresh tokens. HandleLogout must revoke the subject's
// sessions (login + consent, all=true) after a successful accept, on the
// exact path real browser traffic takes (VITE_AUTH_BACKEND_URL points auth-ui
// at this handler, not central-api's own unused LogoutPage).
func TestHandleLogout_RevokesSessionsAfterAccept(t *testing.T) {
	var mu sync.Mutex
	var revokedLogin, revokedConsent string
	var consentQuery string

	hydra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/admin/oauth2/auth/requests/logout"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"challenge":"chal-1","subject":"user-123","client":null}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/logout/accept"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"redirect_to":"https://example.com/logged-out"}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/sessions/login"):
			mu.Lock()
			revokedLogin = r.URL.Query().Get("subject")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/sessions/consent"):
			mu.Lock()
			revokedConsent = r.URL.Query().Get("subject")
			consentQuery = r.URL.RawQuery
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected Hydra call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer hydra.Close()

	hydraClient := service.NewHydraClient(&config.HydraConfig{AdminURL: hydra.URL}, zap.NewNop())
	h := NewLogoutHandler("http://unused.invalid", "internal-secret", hydra.URL, hydraClient, zap.NewNop())

	app := fiber.New()
	app.Get("/logout", h.HandleLogout)

	req := httptest.NewRequest("GET", "/logout?logout_challenge=chal-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want %d (redirect to Hydra's redirect_to)", resp.StatusCode, fiber.StatusFound)
	}

	mu.Lock()
	defer mu.Unlock()
	if revokedLogin != "user-123" {
		t.Errorf("login session revoke subject = %q, want %q", revokedLogin, "user-123")
	}
	if revokedConsent != "user-123" {
		t.Errorf("consent session revoke subject = %q, want %q", revokedConsent, "user-123")
	}
	if !strings.Contains(consentQuery, "all=true") {
		t.Errorf("consent revoke query = %q, want it to include all=true (Single Logout — every client)", consentQuery)
	}
}

// TestHandleLogout_RevocationFailureStillRedirects: revocation is
// best-effort in this browser-facing flow — the user must still be
// redirected back to their app even if Hydra's session-revoke calls fail,
// unlike the authenticated POST /api/v1/logout endpoint whose entire purpose
// is revocation.
func TestHandleLogout_RevocationFailureStillRedirects(t *testing.T) {
	hydra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/admin/oauth2/auth/requests/logout"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"challenge":"chal-1","subject":"user-123","client":null}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/logout/accept"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"redirect_to":"https://example.com/logged-out"}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError) // Hydra having a bad day
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer hydra.Close()

	hydraClient := service.NewHydraClient(&config.HydraConfig{AdminURL: hydra.URL}, zap.NewNop())
	h := NewLogoutHandler("http://unused.invalid", "internal-secret", hydra.URL, hydraClient, zap.NewNop())

	app := fiber.New()
	app.Get("/logout", h.HandleLogout)

	req := httptest.NewRequest("GET", "/logout?logout_challenge=chal-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Errorf("status = %d, want %d — a revoke failure must not block the user's redirect", resp.StatusCode, fiber.StatusFound)
	}
}
