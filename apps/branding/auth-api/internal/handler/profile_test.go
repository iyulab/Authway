package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestGetProfile_RequiresAuthorizationHeader(t *testing.T) {
	h := &ProfileHandler{centralAPIURL: "http://127.0.0.1:1", internalAPIKey: "k", logger: zap.NewNop()}
	app := fiber.New()
	app.Get("/profile/me", h.GetProfile)

	resp, err := app.Test(httptest.NewRequest("GET", "/profile/me", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 without an Authorization header", resp.StatusCode)
	}
}

func TestGetProfile_ProxiesToCentralAPI(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Internal-API-Key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"email":"user@example.com"}`))
	}))
	defer backend.Close()

	h := &ProfileHandler{centralAPIURL: backend.URL, internalAPIKey: "internal-secret", logger: zap.NewNop()}
	app := fiber.New()
	app.Get("/profile/me", h.GetProfile)

	req := httptest.NewRequest("GET", "/profile/me", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"email":"user@example.com"}` {
		t.Errorf("body = %q, want backend response forwarded unchanged", body)
	}

	if gotPath != "/api/v1/profile/me" {
		t.Errorf("backend received path %q, want /api/v1/profile/me", gotPath)
	}
	if gotAuth != "Bearer user-token" {
		t.Errorf("backend received Authorization %q, want it forwarded from the caller", gotAuth)
	}
	if gotAPIKey != "internal-secret" {
		t.Errorf("backend received X-Internal-API-Key %q, want internal-secret", gotAPIKey)
	}
}

func TestGetProfile_BackendUnreachable(t *testing.T) {
	h := &ProfileHandler{centralAPIURL: "http://127.0.0.1:1", internalAPIKey: "k", logger: zap.NewNop()}
	app := fiber.New()
	app.Get("/profile/me", h.GetProfile)

	req := httptest.NewRequest("GET", "/profile/me", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d when Central API is unreachable", resp.StatusCode, http.StatusInternalServerError)
	}
}
