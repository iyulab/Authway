package middleware

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Framework refusals (no route, wrong method) and unexpected errors answer
// like handler refusals: a message in error, a string code.
func TestErrorHandler_AnswersLikeEveryOtherRefusal(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/only-get", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/boom", func(c *fiber.Ctx) error { return errors.New("internal detail") })

	cases := []struct {
		method, path string
		status       int
		code         string
	}{
		{"GET", "/missing", fiber.StatusNotFound, "not_found"},
		{"POST", "/only-get", fiber.StatusMethodNotAllowed, "method_not_allowed"},
		{"GET", "/boom", fiber.StatusInternalServerError, "internal_server_error"},
	}
	for _, tc := range cases {
		resp, err := app.Test(httptest.NewRequest(tc.method, tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tc.status || body["code"] != tc.code || body["error"] == "" || body["error"] == "internal detail" {
			t.Errorf("%s %s: status %d body %v", tc.method, tc.path, resp.StatusCode, body)
		}
	}
}

// A handler that refuses with fiber.NewError gives only a status; the code a
// caller branches on must still say what kind of refusal it was.
func TestErrorHandler_NamesStatusOnlyRefusals(t *testing.T) {
	cases := map[int]string{
		fiber.StatusBadRequest:          "invalid_request",
		fiber.StatusUnauthorized:        "unauthorized",
		fiber.StatusForbidden:           "forbidden",
		fiber.StatusNotFound:            "not_found",
		fiber.StatusConflict:            "conflict",
		fiber.StatusTooManyRequests:     "too_many_requests",
		fiber.StatusInternalServerError: "internal_server_error",
		fiber.StatusBadGateway:          "bad_gateway",
		fiber.StatusServiceUnavailable:  "service_unavailable",
		fiber.StatusGatewayTimeout:      "internal_server_error",
	}
	for status, want := range cases {
		app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
		app.Get("/", func(c *fiber.Ctx) error { return fiber.NewError(status, "refused") })
		resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status || body["code"] != want || body["error"] != "refused" {
			t.Errorf("status %d: got %d %v, want code %s", status, resp.StatusCode, body, want)
		}
	}
}
