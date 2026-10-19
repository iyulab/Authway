package tenantscope

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"authway/apps/central/api/pkg/apierror"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func refusalCode(t *testing.T, err error) (int, string) {
	t.Helper()
	var r *apierror.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("error %v is not a refusal", err)
	}
	return r.Status, r.Code
}

func TestScope_NamedOutsideIsNotFound(t *testing.T) {
	mine, theirs := uuid.New(), uuid.New()
	c := ctxWith(t, theirs.String())
	SetScope(c, Only(map[uuid.UUID]Role{mine: RoleAdmin}))
	_, _, err := Named(c)
	if status, code := refusalCode(t, err); status != fiber.StatusNotFound || code != "not_found" {
		t.Fatalf("Named outside the scope = %d %s, want 404 not_found", status, code)
	}
	if _, err := FromRequest(c); err == nil {
		t.Fatal("FromRequest admitted a tenant outside the scope")
	}
}

func TestScope_OneTenantIsTheOneNamed(t *testing.T) {
	mine := uuid.New()
	c := ctxWith(t, nil)
	SetScope(c, Only(map[uuid.UUID]Role{mine: RoleOwner}))
	got, err := FromRequest(c)
	if err != nil || got != mine {
		t.Fatalf("FromRequest = %v, %v; want the scope's one tenant %v", got, err, mine)
	}
}

func TestScope_SeveralTenantsNeedOneNamed(t *testing.T) {
	c := ctxWith(t, nil)
	SetScope(c, Only(map[uuid.UUID]Role{uuid.New(): RoleOwner, uuid.New(): RoleAdmin}))
	_, err := FromRequest(c)
	if status, code := refusalCode(t, err); status != fiber.StatusBadRequest || code != "tenant_required" {
		t.Fatalf("FromRequest with two tenants and none named = %d %s, want 400 tenant_required", status, code)
	}
}

func TestScope_AdmitRefusesAResourceOutside(t *testing.T) {
	mine, theirs := uuid.New(), uuid.New()
	for _, local := range []any{nil, mine.String()} {
		c := ctxWith(t, local)
		SetScope(c, Only(map[uuid.UUID]Role{mine: RoleAdmin}))
		err := Admit(c, theirs, "client not found")
		if status, code := refusalCode(t, err); status != fiber.StatusNotFound || code != "not_found" {
			t.Fatalf("Admit(another tenant's resource), named %v = %d %s, want 404", local, status, code)
		}
		if err := Admit(c, mine, "client not found"); err != nil {
			t.Fatalf("Admit(own resource), named %v = %v", local, err)
		}
	}
}

// With several tenants and none named, nothing names the tenant: only the
// scope stands between the caller and another tenant's resource.
func TestScope_AdmitRefusesAResourceOutsideSeveral(t *testing.T) {
	a, b, theirs := uuid.New(), uuid.New(), uuid.New()
	c := ctxWith(t, nil)
	SetScope(c, Only(map[uuid.UUID]Role{a: RoleAdmin, b: RoleOwner}))
	err := Admit(c, theirs, "client not found")
	if status, code := refusalCode(t, err); status != fiber.StatusNotFound || code != "not_found" {
		t.Fatalf("Admit(another tenant's resource) = %d %s, want 404", status, code)
	}
	if err := Admit(c, b, "client not found"); err != nil {
		t.Fatalf("Admit(own resource) = %v", err)
	}
}

func TestScope_AllAdmitsWhatTheRequestNames(t *testing.T) {
	owner := uuid.New()
	c := ctxWith(t, nil)
	SetScope(c, All())
	if err := Admit(c, owner, "x"); err != nil {
		t.Fatalf("Admit with every tenant in scope and none named = %v", err)
	}
	if _, ok, err := Named(c); ok || err != nil {
		t.Fatalf("Named with none named = %v, %v; want none", ok, err)
	}
}

// guarded runs handler behind middleware for a request with scope s, the
// tenant header and the route parameter tenant.
func guarded(t *testing.T, s Scope, header string, route string, mw fiber.Handler) int {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if v := c.Get(Header); v != "" {
			c.Locals(LocalKey, v)
		}
		SetScope(c, s)
		return c.Next()
	})
	app.Get(route, mw, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	return func() int {
		path := "/t"
		if route == "/t/:id" {
			path = "/t/" + header
		}
		req := httptest.NewRequest("GET", path, nil)
		if route == "/t" && header != "" {
			req.Header.Set(Header, header)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}()
}

func TestRequireAll(t *testing.T) {
	id := uuid.New()
	if got := guarded(t, All(), "", "/t", RequireAll()); got != fiber.StatusNoContent {
		t.Fatalf("RequireAll with every tenant = %d", got)
	}
	if got := guarded(t, Only(map[uuid.UUID]Role{id: RoleOwner}), "", "/t", RequireAll()); got != fiber.StatusForbidden {
		t.Fatalf("RequireAll for an owner = %d, want 403", got)
	}
}

func TestRequireOwnerOf(t *testing.T) {
	owned, administered, other := uuid.New(), uuid.New(), uuid.New()
	s := Only(map[uuid.UUID]Role{owned: RoleOwner, administered: RoleAdmin})
	for _, tc := range []struct {
		name   string
		tenant uuid.UUID
		want   int
	}{
		{"owner", owned, fiber.StatusNoContent},
		{"admin", administered, fiber.StatusForbidden},
		{"outside", other, fiber.StatusNotFound},
	} {
		if got := guarded(t, s, tc.tenant.String(), "/t/:id", RequireOwnerOf("id")); got != tc.want {
			t.Errorf("%s by route parameter = %d, want %d", tc.name, got, tc.want)
		}
		if got := guarded(t, s, tc.tenant.String(), "/t", RequireOwnerOf("")); got != tc.want {
			t.Errorf("%s by header = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := guarded(t, All(), other.String(), "/t/:id", RequireOwnerOf("id")); got != fiber.StatusNoContent {
		t.Errorf("every tenant in scope = %d, want admitted", got)
	}
}

func TestAdmitParam(t *testing.T) {
	mine, other := uuid.New(), uuid.New()
	s := Only(map[uuid.UUID]Role{mine: RoleAdmin})
	if got := guarded(t, s, mine.String(), "/t/:id", AdmitParam("id")); got != fiber.StatusNoContent {
		t.Errorf("own tenant = %d", got)
	}
	if got := guarded(t, s, other.String(), "/t/:id", AdmitParam("id")); got != fiber.StatusNotFound {
		t.Errorf("another tenant = %d, want 404", got)
	}
}
