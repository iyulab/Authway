package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/user"
)

func connectionsApp(h *AuthHandler, userID uuid.UUID, authTime time.Time) *fiber.App {
	app := fiber.New()
	signedIn := func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		c.Locals("auth_time", authTime)
		return c.Next()
	}
	app.Get("/users/connections", signedIn, h.ListConnections)
	app.Delete("/users/connections/:provider", signedIn, h.RemoveConnection)
	return app
}

func callConnections(t *testing.T, app *fiber.App, method, path string) (int, map[string]any) {
	t.Helper()
	res, err := app.Test(httptest.NewRequest(method, path, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestConnections_ListAndRemove_KeepingAWayToSignIn(t *testing.T) {
	google := "google-subject"
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "linked@example.com", GoogleID: &google}
	h := &AuthHandler{userService: newFakeUserService(u), logger: zap.NewNop()}
	recent := connectionsApp(h, u.ID, time.Now().Add(-time.Minute))

	status, body := callConnections(t, recent, http.MethodGet, "/users/connections")
	list, _ := body["connections"].([]any)
	if status != 200 || len(list) != 1 || list[0].(map[string]any)["provider"] != "google" {
		t.Fatalf("list: %d %v, want google", status, body)
	}

	// Google is the account's only way in: no password.
	if status, body := callConnections(t, recent, http.MethodDelete, "/users/connections/google"); status != 409 || body["code"] != "last_sign_in_method" {
		t.Fatalf("only way in: %d %v, want 409 last_sign_in_method", status, body)
	}
	if status, body := callConnections(t, recent, http.MethodDelete, "/users/connections/github"); status != 404 || body["code"] != "not_connected" {
		t.Fatalf("not linked: %d %v, want 404 not_connected", status, body)
	}

	u.PasswordHash = "set"
	stale := connectionsApp(h, u.ID, time.Now().Add(-time.Hour))
	if status, body := callConnections(t, stale, http.MethodDelete, "/users/connections/google"); status != 401 || body["code"] != "insufficient_user_authentication" {
		t.Fatalf("stale sign-in: %d %v, want 401", status, body)
	}
	if status, body := callConnections(t, recent, http.MethodDelete, "/users/connections/google"); status != 200 {
		t.Fatalf("remove: %d %v, want 200", status, body)
	}
	status, body = callConnections(t, recent, http.MethodGet, "/users/connections")
	if list, _ := body["connections"].([]any); status != 200 || len(list) != 0 {
		t.Fatalf("after removing: %d %v, want none", status, body)
	}
}
