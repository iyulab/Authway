package handler

import (
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestFlowEntry_HandsTheLoginUIAnOpaqueFlowID(t *testing.T) {
	app := fiber.New()
	app.Get("/login", NewFlowEntryHandler(testFrontendURL+"/", nil).Login)

	resp := get(t, app, "/login?login_challenge=abc%3D%3D", "")
	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != testFrontendURL+"/login?flow=abc%3D%3D" {
		t.Fatalf("status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp = get(t, app, "/login", "")
	if resp.StatusCode != fiber.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), testFrontendURL+"/error?error=invalid_request") {
		t.Fatalf("missing challenge: status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// settlerStub settles the flows named in it.
type settlerStub map[string]string

func (s settlerStub) LoginWithoutScreen(_ *fiber.Ctx, flow string) (string, bool) {
	to, ok := s["login:"+flow]
	return to, ok
}

func (s settlerStub) ConsentWithoutScreen(_ *fiber.Ctx, flow string) (string, bool) {
	to, ok := s["consent:"+flow]
	return to, ok
}

// A flow that needs nothing from the user never reaches the login UI.
func TestFlowEntry_SettledFlowsSkipTheScreen(t *testing.T) {
	h := NewFlowEntryHandler(testFrontendURL, settlerStub{
		"login:reused":    "https://issuer.example/after-login",
		"consent:skipped": "https://issuer.example/after-consent",
	})
	app := fiber.New()
	app.Get("/login", h.Login)
	app.Get("/consent", h.Consent)
	app.Get("/logout", h.Logout)

	for path, want := range map[string]string{
		"/login?login_challenge=reused":      "https://issuer.example/after-login",
		"/login?login_challenge=fresh":       testFrontendURL + "/login?flow=fresh",
		"/consent?consent_challenge=skipped": "https://issuer.example/after-consent",
		"/consent?consent_challenge=asked":   testFrontendURL + "/consent?flow=asked",
		// Signing out always asks the screen: it is the user's confirmation.
		"/logout?logout_challenge=reused": testFrontendURL + "/logout?flow=reused",
	} {
		resp := get(t, app, path, "")
		if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != want {
			t.Errorf("%s: status %d location %q, want %q", path, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
}
