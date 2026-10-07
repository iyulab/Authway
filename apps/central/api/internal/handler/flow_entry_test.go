package handler

import (
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestFlowEntry_HandsTheLoginUIAnOpaqueFlowID(t *testing.T) {
	app := fiber.New()
	app.Get("/login", NewFlowEntryHandler(testFrontendURL+"/").Login)

	resp := get(t, app, "/login?login_challenge=abc%3D%3D", "")
	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != testFrontendURL+"/login?flow=abc%3D%3D" {
		t.Fatalf("status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp = get(t, app, "/login", "")
	if resp.StatusCode != fiber.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), testFrontendURL+"/error?error=invalid_request") {
		t.Fatalf("missing challenge: status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}
