package handler

import (
	"bytes"
	"encoding/json"
	"errors"
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
	createFn       func(uuid.UUID, *serviceclient.CreateServiceClientRequest) (*serviceclient.ServiceClient, *serviceclient.ClientCredentials, error)
	revokeFn       func(uuid.UUID, uuid.UUID) error
	listByTenantFn func(uuid.UUID, int, int) ([]*serviceclient.ServiceClient, int64, error)
}

func (f *fakeServiceClientService) Create(tenantID uuid.UUID, req *serviceclient.CreateServiceClientRequest) (*serviceclient.ServiceClient, *serviceclient.ClientCredentials, error) {
	return f.createFn(tenantID, req)
}
func (f *fakeServiceClientService) GetByHydraClientID(string) (*serviceclient.ServiceClient, error) {
	return nil, nil
}
func (f *fakeServiceClientService) ListByTenant(tenantID uuid.UUID, limit, offset int) ([]*serviceclient.ServiceClient, int64, error) {
	if f.listByTenantFn == nil {
		return nil, 0, nil
	}
	return f.listByTenantFn(tenantID, limit, offset)
}
func (f *fakeServiceClientService) Revoke(tenantID, id uuid.UUID) error {
	return f.revokeFn(tenantID, id)
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

func TestServiceClientHandler_List_ScopesToURLTenantAndOmitsSecrets(t *testing.T) {
	tenantID := uuid.New()
	want := []*serviceclient.ServiceClient{
		{ID: uuid.New(), TenantID: tenantID, Name: "svc-a", GrantedScopes: []string{"admin.clients:write"}},
	}

	var gotTenant uuid.UUID
	var gotLimit, gotOffset int
	fake := &fakeServiceClientService{
		listByTenantFn: func(tenant uuid.UUID, limit, offset int) ([]*serviceclient.ServiceClient, int64, error) {
			gotTenant, gotLimit, gotOffset = tenant, limit, offset
			return want, 1, nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Get("/api/v1/tenants/:id/service-clients", h.List)

	req := httptest.NewRequest("GET", "/api/v1/tenants/"+tenantID.String()+"/service-clients", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotTenant != tenantID {
		t.Fatalf("tenantID passed to service.ListByTenant = %v, want %v (must come from the URL)", gotTenant, tenantID)
	}
	if gotLimit != 20 || gotOffset != 0 {
		t.Fatalf("limit/offset = %d/%d, want default 20/0", gotLimit, gotOffset)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := out["client_secret"]; ok {
		t.Fatal("response must never carry a client_secret field")
	}
	scs, ok := out["service_clients"].([]any)
	if !ok || len(scs) != 1 {
		t.Fatalf("expected one service_client in the response, got %+v", out)
	}
}

func TestServiceClientHandler_List_ClampsOutOfRangeLimit(t *testing.T) {
	var gotLimit int
	fake := &fakeServiceClientService{
		listByTenantFn: func(_ uuid.UUID, limit, _ int) ([]*serviceclient.ServiceClient, int64, error) {
			gotLimit = limit
			return nil, 0, nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Get("/api/v1/tenants/:id/service-clients", h.List)

	req := httptest.NewRequest("GET", "/api/v1/tenants/"+uuid.New().String()+"/service-clients?limit=500", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotLimit != 20 {
		t.Fatalf("limit = %d, want clamped default 20 for an out-of-range request", gotLimit)
	}
}

func TestServiceClientHandler_List_InvalidTenantID(t *testing.T) {
	fake := &fakeServiceClientService{
		listByTenantFn: func(uuid.UUID, int, int) ([]*serviceclient.ServiceClient, int64, error) {
			t.Fatal("service.ListByTenant must not be called for an invalid tenant ID")
			return nil, 0, nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Get("/api/v1/tenants/:id/service-clients", h.List)

	req := httptest.NewRequest("GET", "/api/v1/tenants/not-a-uuid/service-clients", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServiceClientHandler_Revoke_ScopesToURLTenant(t *testing.T) {
	tenantID := uuid.New()
	scID := uuid.New()

	var gotTenant, gotID uuid.UUID
	fake := &fakeServiceClientService{
		revokeFn: func(tenant, id uuid.UUID) error {
			gotTenant, gotID = tenant, id
			return nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Delete("/api/v1/tenants/:id/service-clients/:service_client_id", h.Revoke)

	req := httptest.NewRequest("DELETE", "/api/v1/tenants/"+tenantID.String()+"/service-clients/"+scID.String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotTenant != tenantID {
		t.Fatalf("tenantID passed to service.Revoke = %v, want %v (must come from the URL, not ignored)", gotTenant, tenantID)
	}
	if gotID != scID {
		t.Fatalf("id passed to service.Revoke = %v, want %v", gotID, scID)
	}
}

func TestServiceClientHandler_Revoke_InvalidTenantID(t *testing.T) {
	fake := &fakeServiceClientService{
		revokeFn: func(uuid.UUID, uuid.UUID) error {
			t.Fatal("service.Revoke must not be called for an invalid tenant ID")
			return nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Delete("/api/v1/tenants/:id/service-clients/:service_client_id", h.Revoke)

	req := httptest.NewRequest("DELETE", "/api/v1/tenants/not-a-uuid/service-clients/"+uuid.New().String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServiceClientHandler_Revoke_InvalidServiceClientID(t *testing.T) {
	fake := &fakeServiceClientService{
		revokeFn: func(uuid.UUID, uuid.UUID) error {
			t.Fatal("service.Revoke must not be called for an invalid service client ID")
			return nil
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Delete("/api/v1/tenants/:id/service-clients/:service_client_id", h.Revoke)

	req := httptest.NewRequest("DELETE", "/api/v1/tenants/"+uuid.New().String()+"/service-clients/not-a-uuid", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServiceClientHandler_Revoke_ServiceErrorReturnsNotFound(t *testing.T) {
	fake := &fakeServiceClientService{
		revokeFn: func(uuid.UUID, uuid.UUID) error {
			return errors.New("service client not found")
		},
	}
	h := NewServiceClientHandler(fake, zap.NewNop(), nil)

	app := fiber.New()
	app.Delete("/api/v1/tenants/:id/service-clients/:service_client_id", h.Revoke)

	req := httptest.NewRequest("DELETE", "/api/v1/tenants/"+uuid.New().String()+"/service-clients/"+uuid.New().String(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
