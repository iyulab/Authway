package audit

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Exercise ActorFromFiber / EntryFromFiber against a real fiber.Ctx so the
// Locals keyspace stays in sync with what pkg/admin.GetAdminConsoleAuth sets.
// A string/type drift between the middleware and this helper would manifest
// as empty actor fields in audit_logs — invisible in production until an
// incident, so pin the contract in a unit test.
func TestEntryFromFiber_APIKeyFlow(t *testing.T) {
	app := fiber.New()
	tenantID := uuid.New()

	app.Get("/probe", func(c *fiber.Ctx) error {
		// Simulate what admin.GetAdminConsoleAuth sets on the api_key branch.
		c.Locals("actor_type", "api_key")
		c.Locals("actor_key_hint", "abcd1234")
		c.Locals("auth_method", "api_key")

		entry := EntryFromFiber(c, tenantID, ActionClientCreated, "client", "client-id-xyz")

		if entry.ActorType != "api_key" {
			t.Errorf("ActorType = %q, want %q", entry.ActorType, "api_key")
		}
		if entry.ActorID != nil {
			t.Errorf("ActorID = %v, want nil for api_key actor", entry.ActorID)
		}
		if entry.TenantID != tenantID {
			t.Errorf("TenantID mismatch: got %v want %v", entry.TenantID, tenantID)
		}
		if entry.Action != ActionClientCreated {
			t.Errorf("Action = %q, want %q", entry.Action, ActionClientCreated)
		}
		if entry.ResourceType != "client" || entry.ResourceID != "client-id-xyz" {
			t.Errorf("Resource mismatch: %s/%s", entry.ResourceType, entry.ResourceID)
		}
		if hint, ok := entry.Details["actor_key_hint"].(string); !ok || hint != "abcd1234" {
			t.Errorf("actor_key_hint missing/wrong: %v", entry.Details["actor_key_hint"])
		}
		if method, ok := entry.Details["auth_method"].(string); !ok || method != "api_key" {
			t.Errorf("auth_method missing/wrong: %v", entry.Details["auth_method"])
		}
		if !entry.Success {
			t.Error("Success should default to true for write-path success entries")
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("User-Agent", "audit-test/1.0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestEntryFromFiber_AdminSessionFlow(t *testing.T) {
	app := fiber.New()
	tenantID := uuid.New()

	app.Get("/probe", func(c *fiber.Ctx) error {
		c.Locals("actor_type", "admin_session")
		c.Locals("auth_method", "session")

		entry := EntryFromFiber(c, tenantID, ActionClientUpdated, "client", "cid")

		if entry.ActorType != "admin_session" {
			t.Errorf("ActorType = %q, want %q", entry.ActorType, "admin_session")
		}
		if _, exists := entry.Details["actor_key_hint"]; exists {
			t.Error("admin_session flow must not carry actor_key_hint")
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/probe", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
}

func TestEntryFromFiber_MissingLocalsDoesNotPanic(t *testing.T) {
	// If middleware order changes or a route skips adminAuth entirely, the
	// helper must still return a usable entry — audit wiring must never
	// take down the write path.
	app := fiber.New()
	app.Get("/probe", func(c *fiber.Ctx) error {
		entry := EntryFromFiber(c, uuid.New(), ActionClientCreated, "client", "cid")
		if entry.ActorType != "" {
			t.Errorf("ActorType should be empty when no locals set, got %q", entry.ActorType)
		}
		if entry.Details == nil {
			t.Error("Details must never be nil — callers merge into it")
		}
		return c.SendStatus(fiber.StatusOK)
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/probe", nil)); err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
}
