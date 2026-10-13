package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"authway/apps/central/api/pkg/client"
)

// TestStrictSync_FailsLoudlyOnlyWhenAskedTo: a change the authorization server
// did not take is, by default, answered as a success that says so
// (sync_status); a caller that passes strict_sync=true gets a 502 instead.
// The parameter is in the contract for three operations and nothing else
// checked that it does anything.
func TestStrictSync_FailsLoudlyOnlyWhenAskedTo(t *testing.T) {
	h := newTestClientHandler(&fakeClientService{})
	failed := client.SyncStatus{State: client.SyncStateFailed, Error: "authorization server unreachable"}
	ok := client.SyncStatus{State: client.SyncStateOK}

	answer := func(query string, status client.SyncStatus) (int, map[string]any) {
		app := fiber.New()
		app.Put("/clients/:id", func(c *fiber.Ctx) error {
			if code, body := h.respondWithSync(c, status); code != 0 {
				return c.Status(code).JSON(body)
			}
			return c.JSON(fiber.Map{"message": "updated", "sync_status": status})
		})
		res, err := app.Test(httptest.NewRequest("PUT", "/clients/x"+query, nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		raw, _ := io.ReadAll(res.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		return res.StatusCode, body
	}

	if code, body := answer("?strict_sync=true", failed); code != fiber.StatusBadGateway || body["code"] != "upstream_sync_failed" {
		t.Errorf("strict, sync failed: %d %v, want 502 upstream_sync_failed", code, body)
	} else if sync, _ := body["sync_status"].(map[string]any); sync["state"] != client.SyncStateFailed {
		t.Errorf("the refusal does not carry what failed: %v", body)
	}
	if code, body := answer("", failed); code != fiber.StatusOK {
		t.Errorf("not strict, sync failed: %d %v, want 200 with the failure in sync_status", code, body)
	} else if sync, _ := body["sync_status"].(map[string]any); sync["state"] != client.SyncStateFailed {
		t.Errorf("the success does not say the sync failed: %v", body)
	}
	if code, _ := answer("?strict_sync=true", ok); code != fiber.StatusOK {
		t.Errorf("strict, sync succeeded: %d, want 200", code)
	}
	if code, _ := answer("?strict_sync=false", failed); code != fiber.StatusOK {
		t.Errorf("strict_sync=false, sync failed: %d, want 200", code)
	}
}
