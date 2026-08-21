package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/internal/service"
	"authway/apps/central/api/pkg/client"
)

func newTestClientHandler(fake *fakeClientService) *ClientHandler {
	return NewClientHandler(&service.Services{ClientService: fake}, zap.NewNop(), &config.Config{}, nil)
}

// TestCreate_ScopedActor_IgnoresBodyTenantIDAndPrivilegedFields is the core
// security property of the scoped-service-client path: even if the request
// body claims a different tenant_id or sets an admin-only field (e.g.
// apple_private_key), neither reaches client.Service.Create.
func TestCreate_ScopedActor_IgnoresBodyTenantIDAndPrivilegedFields(t *testing.T) {
	scopedTenantID := uuid.New()
	attackerTenantID := uuid.New()

	var gotReq *client.CreateClientRequest
	fake := &fakeClientService{createFn: func(req *client.CreateClientRequest) (*client.Client, *client.ClientCredentials, error) {
		gotReq = req
		return &client.Client{ID: uuid.New(), TenantID: scopedTenantID, ClientID: "authway_x", Name: req.Name}, &client.ClientCredentials{ClientID: "authway_x", ClientSecret: "s"}, nil
	}}
	h := newTestClientHandler(fake)

	app := fiber.New()
	app.Post("/clients", func(c *fiber.Ctx) error {
		c.Locals("actor_type", "service_client")
		c.Locals("tenant_id", scopedTenantID.String())
		return h.Create(c)
	})

	body, _ := json.Marshal(map[string]any{
		"tenant_id":         attackerTenantID.String(), // must be ignored
		"name":              "scoped-app",
		"grant_types":       []string{"authorization_code"},
		"scopes":            []string{"openid"},
		"apple_private_key": "should-never-reach-CreateClientRequest",
	})
	req := httptest.NewRequest("POST", "/clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if gotReq == nil {
		t.Fatal("ClientService.Create was never called")
	}
	if gotReq.TenantID != scopedTenantID.String() {
		t.Fatalf("TenantID = %q, want the scoped credential's tenant %q (body tenant_id must be ignored)", gotReq.TenantID, scopedTenantID.String())
	}
	if gotReq.ApplePrivateKey != "" {
		t.Fatalf("ApplePrivateKey leaked into the scoped request: %q", gotReq.ApplePrivateKey)
	}
}

// TestCreate_ScopedActor_RejectsClientCredentialsGrantType guards that a
// service_client cannot mint another M2M credential through this path.
func TestCreate_ScopedActor_RejectsClientCredentialsGrantType(t *testing.T) {
	fake := &fakeClientService{createFn: func(req *client.CreateClientRequest) (*client.Client, *client.ClientCredentials, error) {
		t.Fatal("ClientService.Create must not be called when validation should reject the request")
		return nil, nil, nil
	}}
	h := newTestClientHandler(fake)

	app := fiber.New()
	app.Post("/clients", func(c *fiber.Ctx) error {
		c.Locals("actor_type", "service_client")
		c.Locals("tenant_id", uuid.New().String())
		return h.Create(c)
	})

	body, _ := json.Marshal(map[string]any{
		"name":        "escalation-attempt",
		"grant_types": []string{"client_credentials"},
		"scopes":      []string{"admin.clients:write"},
	})
	req := httptest.NewRequest("POST", "/clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an out-of-whitelist grant_type", resp.StatusCode)
	}
}

// TestCreate_AdminActor_FullRequestStillWorks is a regression guard: the
// existing full-admin path (no actor_type Local set, as every pre-existing
// caller of adminAuth produces) must be completely unaffected by the branch.
func TestCreate_AdminActor_FullRequestStillWorks(t *testing.T) {
	tenantID := uuid.New()
	var gotReq *client.CreateClientRequest
	fake := &fakeClientService{createFn: func(req *client.CreateClientRequest) (*client.Client, *client.ClientCredentials, error) {
		gotReq = req
		return &client.Client{ID: uuid.New(), TenantID: tenantID, ClientID: "authway_y", Name: req.Name}, &client.ClientCredentials{ClientID: "authway_y", ClientSecret: "s"}, nil
	}}
	h := newTestClientHandler(fake)

	app := fiber.New()
	app.Post("/clients", h.Create) // no actor_type Local — mirrors the existing adminAuth path

	body, _ := json.Marshal(map[string]any{
		"tenant_id":   tenantID.String(),
		"name":        "admin-created",
		"grant_types": []string{"authorization_code"},
		"scopes":      []string{"openid"},
	})
	req := httptest.NewRequest("POST", "/clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if gotReq.TenantID != tenantID.String() {
		t.Fatalf("TenantID = %q, want %q", gotReq.TenantID, tenantID.String())
	}
}
