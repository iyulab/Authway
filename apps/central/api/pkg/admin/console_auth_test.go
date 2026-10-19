package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/console"
	"authway/apps/central/api/pkg/serviceclient"
	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type fakeIntrospector struct{ resp *hydra.IntrospectResponse }

func (f fakeIntrospector) IntrospectToken(string) (*hydra.IntrospectResponse, error) {
	if f.resp == nil {
		return nil, errors.New("unreachable")
	}
	return f.resp, nil
}

type fakeAccounts struct {
	system  uuid.UUID
	members map[uuid.UUID][]console.Membership
}

func (f fakeAccounts) SystemTenantID(context.Context) (uuid.UUID, error) {
	if f.system == uuid.Nil {
		return uuid.Nil, console.ErrNoSystemTenant
	}
	return f.system, nil
}

func (f fakeAccounts) MembershipsOf(_ context.Context, id uuid.UUID) ([]console.Membership, error) {
	return f.members[id], nil
}

func userToken(sub, tenant uuid.UUID, aud ...string) *hydra.IntrospectResponse {
	return &hydra.IntrospectResponse{
		Active: true, Subject: sub.String(), ClientID: "authway-console", Audience: aud,
		Ext: map[string]any{"tenant_id": tenant.String()},
	}
}

// consoleApp answers with the caller's scope: "all", or the tenant it acts on.
func consoleApp(h *Handler, mw fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/protected", mw, func(c *fiber.Ctx) error {
		s, ok := tenantscope.ScopeOf(c)
		if !ok {
			return c.Status(fiber.StatusTeapot).SendString("no scope")
		}
		if s.IsAll() {
			return c.SendString("all")
		}
		id, err := tenantscope.FromRequest(c)
		if err != nil {
			return err
		}
		return c.SendString(id.String())
	})
	return app
}

func call(t *testing.T, app *fiber.App, header string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer console-token")
	if header != "" {
		req.Header.Set(tenantscope.Header, header)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var b [64]byte
	n, _ := res.Body.Read(b[:])
	return res.StatusCode, string(b[:n])
}

func TestConsoleAccount(t *testing.T) {
	system, owned, other := uuid.New(), uuid.New(), uuid.New()
	operator, owner, nobody := uuid.New(), uuid.New(), uuid.New()
	accounts := fakeAccounts{system: system, members: map[uuid.UUID][]console.Membership{
		operator: {{UserID: operator, Role: console.RoleOperator}},
		owner:    {{UserID: owner, TenantID: &owned, Role: "owner"}},
	}}
	cases := []struct {
		name   string
		token  *hydra.IntrospectResponse
		header string
		status int
		body   string
	}{
		{"operator administers every tenant", userToken(operator, system, console.Audience), "", 200, "all"},
		{"owner acts on its tenant", userToken(owner, system, console.Audience), "", 200, owned.String()},
		{"owner naming another tenant", userToken(owner, system, console.Audience), other.String(), 404, ""},
		{"account without memberships", userToken(nobody, system, console.Audience), "", 403, ""},
		{"console audience from another tenant", userToken(operator, other, console.Audience), "", 401, ""},
		{"system tenant token for another application", userToken(operator, system, "https://app.example"), "", 401, ""},
		{"a client's own token", &hydra.IntrospectResponse{Active: true, Subject: "svc", ClientID: "svc", Audience: []string{console.Audience}}, "", 401, ""},
		{"inactive token", &hydra.IntrospectResponse{Active: false, Subject: operator.String(), Audience: []string{console.Audience}}, "", 401, ""},
	}
	delegated := userToken(operator, system, console.Audience)
	delegated.Ext["act"] = map[string]any{"sub": "agent"}
	cases = append(cases, struct {
		name   string
		token  *hydra.IntrospectResponse
		header string
		status int
		body   string
	}{"delegated token", delegated, "", 403, ""})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler("api-key-xyz", nil)
			h.UseConsoleAccounts(fakeIntrospector{tc.token}, accounts)
			status, body := call(t, consoleApp(h, h.GetAdminConsoleAuth()), tc.header)
			if status != tc.status || tc.body != "" && body != tc.body {
				t.Fatalf("GetAdminConsoleAuth = %d %q, want %d %q", status, body, tc.status, tc.body)
			}
		})
	}
}

// Without console accounts wired, a console token is a token like any other.
func TestConsoleAccount_NotWired(t *testing.T) {
	h := newTestHandler("api-key-xyz", nil)
	status, _ := call(t, consoleApp(h, h.GetAdminConsoleAuth()), "")
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

// Every way in sets a scope: a handler never meets an admitted request that
// says nothing about which tenants it may touch.
func TestEveryLegSetsAScope(t *testing.T) {
	system, tenantA := uuid.New(), uuid.New()
	owner := uuid.New()
	accounts := fakeAccounts{system: system, members: map[uuid.UUID][]console.Membership{
		owner: {{UserID: owner, TenantID: &tenantA, Role: "admin"}},
	}}

	h := newTestHandler("api-key-xyz", &stubService{validTokens: map[string]bool{"session-token": true}})
	h.UseConsoleAccounts(fakeIntrospector{userToken(owner, system, console.Audience)}, accounts)
	app := consoleApp(h, h.GetAdminConsoleAuth())
	for _, token := range []string{"api-key-xyz", "session-token", "console-token"} {
		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != fiber.StatusOK {
			t.Errorf("%s: status %d — admitted without a scope, or refused", token, res.StatusCode)
		}
	}

	sc := "authway_svc_scope-test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(hydra.IntrospectResponse{Active: true, Subject: sc, ClientID: sc})
	}))
	t.Cleanup(srv.Close)
	svc := &fakeServiceClientService{byHydraClientID: map[string]*serviceclient.ServiceClient{
		sc: {ID: uuid.New(), TenantID: tenantA, HydraClientID: sc, GrantedScopes: []string{"admin.claims:write"}},
	}}
	scApp := consoleApp(h, h.GetClientAuth(hydra.NewClient(srv.URL), svc, "admin.claims:write"))
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer service-token")
	res, err := scApp.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Errorf("service client: status %d — admitted without a scope, or refused", res.StatusCode)
	}
}
