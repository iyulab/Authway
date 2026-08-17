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

type claimsRoute struct {
	name              string
	method            string
	route             string
	handler           fiber.Handler
	wantBackendPath   string
	wantBackendMethod string
}

func claimsRoutes(h *ClaimsHandler) []claimsRoute {
	return []claimsRoute{
		{"GetClaims", http.MethodGet, "/claims", h.GetClaims, "/api/v1/claims", http.MethodGet},
		{"UpdateClaims", http.MethodPatch, "/claims", h.UpdateClaims, "/api/v1/claims", http.MethodPatch},
		{"GetUserClaims", http.MethodGet, "/claims/user", h.GetUserClaims, "/api/v1/claims/user", http.MethodGet},
		{"UpdateUserClaims", http.MethodPatch, "/claims/user", h.UpdateUserClaims, "/api/v1/claims/user", http.MethodPatch},
	}
}

func registerRoute(app *fiber.App, tt claimsRoute) {
	switch tt.method {
	case http.MethodGet:
		app.Get(tt.route, tt.handler)
	case http.MethodPatch:
		app.Patch(tt.route, tt.handler)
	}
}

func TestClaimsHandler_RequiresAuthorizationHeader(t *testing.T) {
	// Backend must never be reached without an Authorization header — the
	// handler is expected to reject before proxying.
	h := &ClaimsHandler{centralAPIURL: "http://127.0.0.1:1", internalAPIKey: "k", logger: zap.NewNop()}

	for _, tt := range claimsRoutes(h) {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			registerRoute(app, tt)

			req := httptest.NewRequest(tt.method, tt.route, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test error: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401 without an Authorization header", resp.StatusCode)
			}
		})
	}
}

func TestClaimsHandler_ProxiesToCentralAPI(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotAPIKey, gotContentType string
	var gotBody []byte

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Internal-API-Key")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	h := &ClaimsHandler{centralAPIURL: backend.URL, internalAPIKey: "internal-secret", logger: zap.NewNop()}

	for _, tt := range claimsRoutes(h) {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			registerRoute(app, tt)

			var body io.Reader
			if tt.method == http.MethodPatch {
				body = strings.NewReader(`{"claim":"value"}`)
			}
			req := httptest.NewRequest(tt.method, tt.route, body)
			req.Header.Set("Authorization", "Bearer user-token")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
			respBody, _ := io.ReadAll(resp.Body)
			if string(respBody) != `{"ok":true}` {
				t.Errorf("body = %q, want backend response forwarded unchanged", respBody)
			}

			if gotPath != tt.wantBackendPath {
				t.Errorf("backend received path %q, want %q", gotPath, tt.wantBackendPath)
			}
			if gotMethod != tt.wantBackendMethod {
				t.Errorf("backend received method %q, want %q", gotMethod, tt.wantBackendMethod)
			}
			if gotAuth != "Bearer user-token" {
				t.Errorf("backend received Authorization %q, want it forwarded from the caller", gotAuth)
			}
			if gotAPIKey != "internal-secret" {
				t.Errorf("backend received X-Internal-API-Key %q, want internal-secret", gotAPIKey)
			}
			if tt.method == http.MethodPatch {
				if gotContentType != "application/json" {
					t.Errorf("backend received Content-Type %q, want application/json", gotContentType)
				}
				if string(gotBody) != `{"claim":"value"}` {
					t.Errorf("backend received body %q, want request body forwarded unchanged", gotBody)
				}
			}
		})
	}
}

func TestClaimsHandler_BackendUnreachable(t *testing.T) {
	h := &ClaimsHandler{centralAPIURL: "http://127.0.0.1:1", internalAPIKey: "k", logger: zap.NewNop()}

	for _, tt := range claimsRoutes(h) {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			registerRoute(app, tt)

			req := httptest.NewRequest(tt.method, tt.route, nil)
			req.Header.Set("Authorization", "Bearer user-token")
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
