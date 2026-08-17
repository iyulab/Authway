package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestAuthenticate_ProxiesToCentralAPI(t *testing.T) {
	var gotMethod, gotPath, gotContentType, gotAPIKey string
	var gotBody []byte

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotAPIKey = r.Header.Get("X-Internal-API-Key")
		gotBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"access_token":"tok"}`))
	}))
	defer backend.Close()

	h := &AuthHandler{centralAPIURL: backend.URL, internalAPIKey: "internal-secret", logger: zap.NewNop()}
	app := fiber.New()
	app.Post("/authenticate", h.Authenticate)

	req := httptest.NewRequest("POST", "/authenticate", strings.NewReader(`{"email":"a@b.com"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"access_token":"tok"}` {
		t.Errorf("body = %q, want backend response forwarded unchanged", body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("backend received method %q, want POST", gotMethod)
	}
	if gotPath != "/authenticate" {
		t.Errorf("backend received path %q, want /authenticate", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("backend received Content-Type %q, want application/json", gotContentType)
	}
	if gotAPIKey != "internal-secret" {
		t.Errorf("backend received X-Internal-API-Key %q, want internal-secret", gotAPIKey)
	}
	if string(gotBody) != `{"email":"a@b.com"}` {
		t.Errorf("backend received body %q, want request body forwarded unchanged", gotBody)
	}
}

func TestAuthenticate_ForwardsBackendErrorStatus(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_credentials"}`))
	}))
	defer backend.Close()

	h := &AuthHandler{centralAPIURL: backend.URL, internalAPIKey: "k", logger: zap.NewNop()}
	app := fiber.New()
	app.Post("/authenticate", h.Authenticate)

	req := httptest.NewRequest("POST", "/authenticate", strings.NewReader(`{}`))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want backend's 401 forwarded unchanged", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"error":"invalid_credentials"}` {
		t.Errorf("body = %q, want backend error body forwarded unchanged", body)
	}
}

func TestAuthenticate_BackendUnreachable(t *testing.T) {
	// Port 1 is unassigned/privileged and refuses connections immediately on
	// loopback, giving a deterministic "backend down" case with no real network.
	h := &AuthHandler{centralAPIURL: "http://127.0.0.1:1", internalAPIKey: "k", logger: zap.NewNop()}
	app := fiber.New()
	app.Post("/authenticate", h.Authenticate)

	req := httptest.NewRequest("POST", "/authenticate", strings.NewReader(`{}`))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d when Central API is unreachable", resp.StatusCode, http.StatusInternalServerError)
	}
}
