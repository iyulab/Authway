package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// consentRoutes enumerates the three proxy methods so both success and
// failure behavior can be verified once for all of them instead of
// triplicating near-identical test bodies — the handlers themselves are
// near-identical proxy bodies too.
func consentRoutes(h *ConsentHandler) []struct {
	name            string
	route           string
	handler         fiber.Handler
	wantBackendPath string
} {
	return []struct {
		name            string
		route           string
		handler         fiber.Handler
		wantBackendPath string
	}{
		{"GetConsentInfo", "/consent", h.GetConsentInfo, "/consent"},
		{"AcceptConsent", "/consent/accept", h.AcceptConsent, "/consent/accept"},
		{"RejectConsent", "/consent/reject", h.RejectConsent, "/consent/reject"},
	}
}

func TestConsentHandler_ProxiesToCentralAPI(t *testing.T) {
	var gotPath, gotAPIKey, gotContentType string
	var gotBody []byte

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-Internal-API-Key")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	h := &ConsentHandler{centralAPIURL: backend.URL, internalAPIKey: "internal-secret", logger: zap.NewNop()}

	for _, tt := range consentRoutes(h) {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Post(tt.route, tt.handler)

			req := httptest.NewRequest("POST", tt.route, strings.NewReader(`{"challenge":"c1"}`))
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != `{"ok":true}` {
				t.Errorf("body = %q, want backend response forwarded unchanged", body)
			}

			if gotPath != tt.wantBackendPath {
				t.Errorf("backend received path %q, want %q", gotPath, tt.wantBackendPath)
			}
			if gotAPIKey != "internal-secret" {
				t.Errorf("backend received X-Internal-API-Key %q, want internal-secret", gotAPIKey)
			}
			if gotContentType != "application/json" {
				t.Errorf("backend received Content-Type %q, want application/json", gotContentType)
			}
			if string(gotBody) != `{"challenge":"c1"}` {
				t.Errorf("backend received body %q, want request body forwarded unchanged", gotBody)
			}
		})
	}
}

func TestConsentHandler_BackendUnreachable(t *testing.T) {
	h := &ConsentHandler{centralAPIURL: "http://127.0.0.1:1", internalAPIKey: "k", logger: zap.NewNop()}

	for _, tt := range consentRoutes(h) {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Post(tt.route, tt.handler)

			req := httptest.NewRequest("POST", tt.route, strings.NewReader(`{}`))
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test error: %v", err)
			}
			if resp.StatusCode != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d when Central API is unreachable", resp.StatusCode, http.StatusInternalServerError)
			}
		})
	}
}
