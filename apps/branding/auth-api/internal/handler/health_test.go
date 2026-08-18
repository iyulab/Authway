package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHealth(t *testing.T) {
	h := NewHealthHandler("v20260818-test")
	app := fiber.New()
	app.Get("/health", h.Health)

	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to decode response: %v (body=%s)", err, body)
	}
	if got["status"] != "OK" {
		t.Errorf("status field = %v, want OK", got["status"])
	}
	if got["version"] != "v20260818-test" {
		t.Errorf("version field = %v, want the build-time value", got["version"])
	}
}

// TestConfig_ReportsBuildVersion is the regression this cycle exists for:
// /.well-known/authway-config used to hard-code "version": "0.1.0" — a
// literal, not tied to the build at all — so it never reflected what was
// actually deployed no matter what image was running.
func TestConfig_ReportsBuildVersion(t *testing.T) {
	h := NewHealthHandler("v20260818-test")
	app := fiber.New()
	app.Get("/config", h.Config)

	resp, err := app.Test(httptest.NewRequest("GET", "/config", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &got)

	if got["version"] != "v20260818-test" {
		t.Errorf("version = %v, want the build-time value, not a hard-coded literal", got["version"])
	}
}

func TestConfig_Defaults(t *testing.T) {
	os.Unsetenv("HYDRA_PUBLIC_URL")
	os.Unsetenv("AUTH_BACKEND_URL")

	h := NewHealthHandler("v20260818-test")
	app := fiber.New()
	app.Get("/config", h.Config)

	resp, err := app.Test(httptest.NewRequest("GET", "/config", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &got)

	if got["oauth_url"] != "http://localhost:4444" {
		t.Errorf("oauth_url = %v, want the default", got["oauth_url"])
	}
	if got["api_url"] != "http://localhost:8081" {
		t.Errorf("api_url = %v, want the default", got["api_url"])
	}
	if got["issuer"] != got["oauth_url"] {
		t.Errorf("issuer = %v, want it to equal oauth_url (same Hydra public endpoint)", got["issuer"])
	}
}

func TestConfig_EnvOverride(t *testing.T) {
	os.Setenv("HYDRA_PUBLIC_URL", "https://hydra.example.com")
	os.Setenv("AUTH_BACKEND_URL", "https://auth.example.com")
	defer os.Unsetenv("HYDRA_PUBLIC_URL")
	defer os.Unsetenv("AUTH_BACKEND_URL")

	h := NewHealthHandler("v20260818-test")
	app := fiber.New()
	app.Get("/config", h.Config)

	resp, err := app.Test(httptest.NewRequest("GET", "/config", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &got)

	if got["oauth_url"] != "https://hydra.example.com" {
		t.Errorf("oauth_url = %v, want the env override", got["oauth_url"])
	}
	if got["api_url"] != "https://auth.example.com" {
		t.Errorf("api_url = %v, want the env override", got["api_url"])
	}
}
