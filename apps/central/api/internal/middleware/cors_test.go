package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
)

func TestCORSPreflightAllowsTheTenantHeader(t *testing.T) {
	const origin = "https://admin.example.com"
	app := fiber.New()
	app.Use(CORS([]string{origin}))
	app.Get("/api/v1/webhooks", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(fiber.MethodOptions, "/api/v1/webhooks", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", fiber.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization,"+strings.ToLower(tenantscope.Header))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, origin)
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
