package claims

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeUsers map[uuid.UUID]*user.User

func (f fakeUsers) GetByID(id uuid.UUID) (*user.User, error) {
	if u, ok := f[id]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

// patchOnly stores nothing; it answers a patch the way the service checks it.
type patchOnly struct{ patched []uuid.UUID }

func (p *patchOnly) List(context.Context, uuid.UUID, uuid.UUID) (ClaimMap, error) {
	return ClaimMap{}, nil
}
func (p *patchOnly) Patch(_ context.Context, userID, _ uuid.UUID, patch map[string]any) (ClaimMap, error) {
	if err := checkWritable(ClaimMap(patch)); err != nil {
		return nil, err
	}
	p.patched = append(p.patched, userID)
	return ClaimMap(patch), nil
}

// A tenant's service credential reaches only that tenant's users: a user of
// another tenant is not found, the same answer as for an id that does not
// exist. A caller that names no tenant (the admin key) reaches every user.
func TestHandler_ScopesToTheCallersTenant(t *testing.T) {
	tenantA, tenantB := uuid.New(), uuid.New()
	inA := &user.User{ID: uuid.New(), TenantID: tenantA}
	inB := &user.User{ID: uuid.New(), TenantID: tenantB}
	svc := &patchOnly{}
	h := NewHandler(svc, fakeUsers{inA.ID: inA, inB.ID: inB}, nil, zap.NewNop())

	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		var r *apierror.Refusal
		if errors.As(err, &r) {
			return apierror.Refuse(c, r.Status, r.Code, r.Message)
		}
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", err.Error())
	}})
	app.Use(func(c *fiber.Ctx) error {
		if tenant := c.Get("X-Test-Tenant"); tenant != "" {
			c.Locals("tenant_id", tenant)
		}
		return c.Next()
	})
	app.Patch("/users/:id/claims", h.Patch)

	patch := func(id uuid.UUID, tenant, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/users/"+id.String()+"/claims", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if tenant != "" {
			req.Header.Set("X-Test-Tenant", tenant)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out
	}

	if status, body := patch(inA.ID, tenantA.String(), `{"claims":{"plan":"pro"}}`); status != 200 {
		t.Fatalf("own tenant: %d %v", status, body)
	}
	if status, body := patch(inB.ID, tenantA.String(), `{"claims":{"plan":"pro"}}`); status != 404 || body["code"] != "not_found" {
		t.Fatalf("other tenant: want 404 not_found, got %d %v", status, body)
	}
	if status, body := patch(uuid.New(), tenantA.String(), `{"claims":{"plan":"pro"}}`); status != 404 || body["code"] != "not_found" {
		t.Fatalf("unknown user: want 404 not_found, got %d %v", status, body)
	}
	if status, body := patch(inB.ID, "", `{"claims":{"plan":"pro"}}`); status != 200 {
		t.Fatalf("no tenant named (admin key): %d %v", status, body)
	}
	if status, body := patch(inA.ID, tenantA.String(), `{"claims":{"tenant_id":"x"}}`); status != 400 || body["code"] != "reserved_claim" {
		t.Fatalf("reserved: want 400 reserved_claim, got %d %v", status, body)
	}
	if status, body := patch(inA.ID, tenantA.String(), `{"plan":"pro"}`); status != 400 || body["code"] != "invalid_request" {
		t.Fatalf("no claims object: want 400 invalid_request, got %d %v", status, body)
	}
	if len(svc.patched) != 2 || svc.patched[0] != inA.ID || svc.patched[1] != inB.ID {
		t.Fatalf("patched users: %v", svc.patched)
	}
}
