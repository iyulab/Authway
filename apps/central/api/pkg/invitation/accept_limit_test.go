package invitation

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// Accepting an invitation has the limit it is given in front of it, and the
// admin routes keep their own middleware, not that limit.
func TestRegisterRoutes_AcceptingAnInvitationIsLimited(t *testing.T) {
	app := fiber.New()
	refuse := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusTooManyRequests) }
	admin := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) }
	NewHandler(nil, zap.NewNop()).RegisterRoutes(app.Group("/api/v1"), refuse, admin)

	for _, route := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/api/v1/invitations/accept", fiber.StatusTooManyRequests},
		{"GET", "/api/v1/invitations", fiber.StatusUnauthorized},
	} {
		res, err := app.Test(httptest.NewRequest(route.method, route.path, nil), -1)
		if err != nil {
			t.Fatalf("%s %s: %v", route.method, route.path, err)
		}
		if res.StatusCode != route.want {
			t.Errorf("%s %s: status %d, want %d", route.method, route.path, res.StatusCode, route.want)
		}
	}
}
