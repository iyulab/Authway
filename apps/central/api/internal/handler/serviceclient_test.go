package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/serviceclient"
)

// fakeServiceClientService is a minimal in-memory serviceclient.Service for
// handler tests — controls Create/Revoke outcomes without a real DB or Hydra.
type fakeServiceClientService struct {
	createFn func(uuid.UUID, *serviceclient.CreateServiceClientRequest) (*serviceclient.ServiceClient, *serviceclient.ClientCredentials, error)
	revokeFn func(uuid.UUID) error
}

func (f *fakeServiceClientService) Create(tenantID uuid.UUID, req *serviceclient.CreateServiceClientRequest) (*serviceclient.ServiceClient, *serviceclient.ClientCredentials, error) {
	return f.createFn(tenantID, req)
}
func (f *fakeServiceClientService) GetByHydraClientID(string) (*serviceclient.ServiceClient, error) {
	return nil, nil
}
func (f *fakeServiceClientService) Revoke(id uuid.UUID) error {
	return f.revokeFn(id)
}

func TestServiceClientHandler_Create_ReturnsCredentialsOnce(t *testing.T) {
	tenantID := uuid.New()
	wantSC := &serviceclient.ServiceClient{ID: uuid.New(), TenantID: tenantID, Name: "scoped-svc", GrantedScopes: []string{"admin.clients:write"}}
	wantCreds := &serviceclient.ClientCredentials{ClientID: "authway_svc_abc", ClientSecret: "s3cr3t"}

	fake := &fakeServiceClientService{
		createFn: func(gotTenant uuid.UUID, req *serviceclient.CreateServiceClientRequest) (*serviceclient.ServiceClient, *serviceclient.ClientCredentials, error) {
			if gotTenant != tenantID {
				t.Fatalf("tenantID = %v, want %v (must come from the URL param, not the body)", gotTenant, tenantID)
			}
			return wantSC, wantCreds, nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Post("/api/v1/tenants/:id/service-clients", h.Create)

	body, _ := json.Marshal(map[string]any{"name": "scoped-svc", "scopes": []string{"admin.clients:write"}})
	req := httptest.NewRequest("POST", "/api/v1/tenants/"+tenantID.String()+"/service-clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	creds, ok := out["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected a credentials object in the response, got %+v", out)
	}
	if creds["client_secret"] != "s3cr3t" {
		t.Fatalf("expected the raw secret in the create response, got %+v", creds)
	}
}

func TestServiceClientHandler_Create_InvalidTenantID(t *testing.T) {
	fake := &fakeServiceClientService{}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Post("/api/v1/tenants/:id/service-clients", h.Create)

	body, _ := json.Marshal(map[string]any{"name": "x", "scopes": []string{"admin.clients:write"}})
	req := httptest.NewRequest("POST", "/api/v1/tenants/not-a-uuid/service-clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
