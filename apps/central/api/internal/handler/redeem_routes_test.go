package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// refuse stands in for a limiter that has blocked the caller: a route that has
// it in front answers 429 without reaching its handler.
func refuse(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusTooManyRequests) }

// The routes that redeem a reset link have the redemption limit in front.
func TestEmailRoutes_RedeemingAResetLinkIsLimited(t *testing.T) {
	app := fiber.New()
	(&EmailHandler{}).RegisterRoutes(app.Group("/api/v1"), refuse)
	for _, route := range []struct{ method, path string }{
		{"POST", "/api/v1/email/reset-password"},
		{"GET", "/api/v1/email/verify-reset-token"},
	} {
		res, err := app.Test(httptest.NewRequest(route.method, route.path, nil), -1)
		if err != nil {
			t.Fatalf("%s %s: %v", route.method, route.path, err)
		}
		if res.StatusCode != fiber.StatusTooManyRequests {
			t.Errorf("%s %s: status %d, want 429 from the limit in front of it", route.method, route.path, res.StatusCode)
		}
	}
}
