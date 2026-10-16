package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
)

const (
	consoleOrigin    = "https://admin.example.com"
	registeredOrigin = "https://app.example.org"
	strangerOrigin   = "https://stranger.example.net"
)

// newCORSApp serves one route of each kind behind CORS, with the console as
// the deployment's own origin and registeredOrigin registered on a client.
func newCORSApp() *fiber.App {
	app := fiber.New()
	app.Use(CORS([]string{consoleOrigin}, func(origin string) bool { return origin == registeredOrigin }))
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Get("/.well-known/authway-config", ok)
	app.Get("/api/v1/capabilities", ok)
	app.Get("/api/v1/profile/me", ok)
	app.Post("/api/v1/users/mfa/setup", ok)
	app.Post("/api/v1/logout", ok)
	app.Post("/api/v1/logout-flows/:id/accept", ok)
	app.Get("/api/v1/webhooks", ok)
	app.Post("/oauth2/token", ok)
	return app
}

func preflight(t *testing.T, app *fiber.App, method, path, origin string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodOptions, path, nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", method)
	req.Header.Set("Access-Control-Request-Headers", "authorization,"+strings.ToLower(tenantscope.Header))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestCORSPreflightAllowsTheTenantHeader(t *testing.T) {
	resp := preflight(t, newCORSApp(), fiber.MethodGet, "/api/v1/webhooks", consoleOrigin)
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != consoleOrigin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, consoleOrigin)
	}
	allowed := strings.Split(strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers")), ",")
	for _, want := range []string{"authorization", strings.ToLower(tenantscope.Header)} {
		found := false
		for _, h := range allowed {
			if strings.TrimSpace(h) == want {
				found = true
			}
		}
		if !found {
			t.Errorf("preflight does not allow %q; allowed: %v", want, allowed)
		}
	}
}

// The deployment's own screens call every route and send cookies.
func TestCORS_TheDeploymentsOwnOriginMaySendCookiesEverywhere(t *testing.T) {
	app := newCORSApp()
	for _, path := range []string{"/api/v1/webhooks", "/api/v1/profile/me", "/api/v1/logout-flows/x/accept"} {
		resp := preflight(t, app, fiber.MethodPost, path, consoleOrigin)
		if resp.Header.Get("Access-Control-Allow-Origin") != consoleOrigin || resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("%s: origin %q credentials %q, want the console admitted with credentials", path,
				resp.Header.Get("Access-Control-Allow-Origin"), resp.Header.Get("Access-Control-Allow-Credentials"))
		}
	}
}

// An application's page reaches the routes its users call with their own
// token — and only those, and never with cookies.
func TestCORS_ARegisteredOriginReachesTheUserTokenRoutesOnly(t *testing.T) {
	app := newCORSApp()
	for path, method := range map[string]string{
		"/api/v1/profile/me":      fiber.MethodGet,
		"/api/v1/users/mfa/setup": fiber.MethodPost,
		"/api/v1/logout":          fiber.MethodPost,
	} {
		resp := preflight(t, app, method, path, registeredOrigin)
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != registeredOrigin {
			t.Errorf("%s: Access-Control-Allow-Origin = %q, want the registered origin", path, got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("%s: Access-Control-Allow-Credentials = %q, want none for a registered origin", path, got)
		}
		if !strings.Contains(strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers")), "authorization") {
			t.Errorf("%s: the preflight does not allow the Authorization header", path)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			t.Errorf("%s: preflight status %d, want 204", path, resp.StatusCode)
		}
	}

	for _, path := range []string{"/api/v1/webhooks", "/api/v1/logout-flows/x/accept"} {
		if got := preflight(t, app, fiber.MethodPost, path, registeredOrigin).Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s: a registered origin was admitted (%q) to a route that is not a user-token route", path, got)
		}
	}

	// The answer itself carries the header too, or the page cannot read it.
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/profile/me", nil)
	req.Header.Set("Origin", registeredOrigin)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != registeredOrigin {
		t.Errorf("GET: status %d origin %q, want 200 with the registered origin", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_AnUnregisteredOriginReachesOnlyThePublicDocuments(t *testing.T) {
	app := newCORSApp()
	for _, path := range []string{"/api/v1/profile/me", "/api/v1/webhooks", "/api/v1/logout"} {
		if got := preflight(t, app, fiber.MethodGet, path, strangerOrigin).Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s: an unregistered origin was admitted (%q)", path, got)
		}
	}
	for _, path := range publicDocuments {
		req := httptest.NewRequest(fiber.MethodGet, path, nil)
		req.Header.Set("Origin", strangerOrigin)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: status %d origin %q, want 200 readable from any origin", path, resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
		}
		if resp.Header.Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("%s: a public document must not allow credentials", path)
		}
	}
}

func TestCORS_AnyOriginExchangesAtTheTokenEndpointWithoutCookies(t *testing.T) {
	app := newCORSApp()
	resp := preflight(t, app, fiber.MethodPost, "/oauth2/token", strangerOrigin)
	if resp.StatusCode != fiber.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("preflight: status %d origin %q, want 204 from any origin", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	req := httptest.NewRequest(fiber.MethodPost, "/oauth2/token", nil)
	req.Header.Set("Origin", strangerOrigin)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("POST: status %d origin %q, want 200 readable from any origin", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if resp.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Error("the token endpoint must not allow credentials")
	}
}

func TestRememberOrigins(t *testing.T) {
	calls := map[string]int{}
	var fail error
	registered := RememberOrigins(func(origin string) (bool, error) {
		calls[origin]++
		return origin == registeredOrigin, fail
	}, 50*time.Millisecond)

	for i := 0; i < 3; i++ {
		if !registered(registeredOrigin) || registered(strangerOrigin) {
			t.Fatal("wrong answer")
		}
	}
	if calls[registeredOrigin] != 1 || calls[strangerOrigin] != 1 {
		t.Errorf("looked up %v, want one lookup per origin while the answer is remembered", calls)
	}

	time.Sleep(80 * time.Millisecond)
	registered(registeredOrigin)
	if calls[registeredOrigin] != 2 {
		t.Errorf("looked up %d times after the answer expired, want 2", calls[registeredOrigin])
	}

	// A lookup that fails refuses the origin and is asked again next time.
	fail = errors.New("database is down")
	const flaky = "https://flaky.example.org"
	if registered(flaky) {
		t.Error("an origin was admitted although the lookup failed")
	}
	registered(flaky)
	if calls[flaky] != 2 {
		t.Errorf("a failed lookup was remembered (looked up %d times, want 2)", calls[flaky])
	}
}
