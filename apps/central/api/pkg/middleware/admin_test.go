package middleware

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// decodeJSONError returns the `error` field from a JSON response body.
func decodeJSONError(t *testing.T, body io.Reader) string {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if v, ok := payload["error"].(string); ok {
		return v
	}
	return ""
}

func newAppWithAdminAuth(apiKey string) *fiber.App {
	app := fiber.New()
	app.Get("/protected", AdminAuth(apiKey), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})
	return app
}

// TestAdminAuth_FailClosed_EmptyKey verifies that a missing/empty admin API key
// causes every request to be rejected with 503 — the opposite of the pre-0.2.1
// silent bypass that allowed unauthenticated access to client CRUD in
// production deployments without AUTHWAY_ADMIN_API_KEY set.
func TestAdminAuth_FailClosed_EmptyKey(t *testing.T) {
	app := newAppWithAdminAuth("")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer anything")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("want 503 (fail-closed), got %d", resp.StatusCode)
	}
	if msg := decodeJSONError(t, resp.Body); msg == "" {
		t.Fatalf("expected error message in body")
	}
}

func TestAdminAuth_MissingAuthorizationHeader(t *testing.T) {
	app := newAppWithAdminAuth("secret")

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_InvalidFormat(t *testing.T) {
	app := newAppWithAdminAuth("secret")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "secret") // missing Bearer prefix
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_InvalidKey(t *testing.T) {
	app := newAppWithAdminAuth("secret")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_ValidKey(t *testing.T) {
	app := newAppWithAdminAuth("secret")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestRequireAdmin_WithoutAdminAuth(t *testing.T) {
	app := fiber.New()
	app.Get("/x", RequireAdmin(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/x", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("want 403, got %d", resp.StatusCode)
	}
}
