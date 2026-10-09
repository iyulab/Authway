package tenantscope

import (
	"errors"
	"testing"

	"authway/apps/central/api/pkg/apierror"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

func ctxWith(t *testing.T, value any) *fiber.Ctx {
	t.Helper()
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	t.Cleanup(func() { app.ReleaseCtx(c) })
	if value != nil {
		c.Locals(LocalKey, value)
	}
	return c
}

func TestFromRequest(t *testing.T) {
	id := uuid.New()
	cases := []struct {
		name  string
		local any
		want  uuid.UUID
		code  string
	}{
		{"admin middleware string", id.String(), id, ""},
		{"access token uuid", id, id, ""},
		{"nothing named", nil, uuid.Nil, "tenant_required"},
		{"empty string", "", uuid.Nil, "tenant_required"},
		{"nil uuid", uuid.Nil, uuid.Nil, "tenant_required"},
		{"not an id", "acme", uuid.Nil, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromRequest(ctxWith(t, tc.local))
			if tc.code == "" {
				if err != nil || got != tc.want {
					t.Fatalf("FromRequest = %v, %v; want %v", got, err, tc.want)
				}
				return
			}
			var refusal *apierror.Refusal
			if !errors.As(err, &refusal) || refusal.Code != tc.code || refusal.Status != fiber.StatusBadRequest {
				t.Fatalf("FromRequest error = %v, want 400 %s", err, tc.code)
			}
		})
	}
}

func TestAdmit(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	cases := []struct {
		name   string
		local  any
		status int
		code   string
	}{
		{"names the owner", owner.String(), 0, ""},
		{"token names the owner", owner, 0, ""},
		{"names nothing", nil, 0, ""},
		{"names another tenant", other.String(), fiber.StatusNotFound, "not_found"},
		{"token names another tenant", other, fiber.StatusNotFound, "not_found"},
		{"names something that is not an id", "acme", fiber.StatusBadRequest, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Admit(ctxWith(t, tc.local), owner, "webhook not found")
			if tc.code == "" {
				if err != nil {
					t.Fatalf("Admit = %v, want admitted", err)
				}
				return
			}
			var refusal *apierror.Refusal
			if !errors.As(err, &refusal) || refusal.Code != tc.code || refusal.Status != tc.status {
				t.Fatalf("Admit error = %v, want %d %s", err, tc.status, tc.code)
			}
		})
	}
}
