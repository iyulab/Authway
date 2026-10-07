package handler

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"authway/apps/central/api/internal/hydra"
)

func TestRespondFlowLookupError_StatusByCause(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{fmt.Errorf("wrapped: %w", hydra.ErrFlowNotFound), fiber.StatusBadRequest},
		{fmt.Errorf("wrapped: %w", hydra.ErrFlowExpired), fiber.StatusGone},
		{fmt.Errorf("dial tcp: connection refused"), fiber.StatusBadGateway},
	}
	for _, tc := range cases {
		app := fiber.New()
		app.Get("/", func(c *fiber.Ctx) error { return respondFlowLookupError(c, tc.err) })
		resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != tc.want {
			t.Errorf("%v: status = %d, want %d (body %s)", tc.err, resp.StatusCode, tc.want, body)
		}
		// The raw cause (internal addresses, Hydra's own messages) must not leak.
		if got := string(body); got == "" || strings.Contains(got, "connection refused") || strings.Contains(got, "wrapped") {
			t.Errorf("%v: response leaks the internal cause: %s", tc.err, got)
		}
	}
}
