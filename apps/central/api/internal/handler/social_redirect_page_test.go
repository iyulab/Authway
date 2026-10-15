package handler

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// The page that carries the browser on after a provider sign-in must keep the
// address inside the script's string, whatever the address contains.
func TestRenderRedirectPageEscapesTheAddress(t *testing.T) {
	h := &SocialHandler{logger: zap.NewNop()}
	app := fiber.New()
	const hostile = `https://issuer.example/oauth2/auth?state=x";</script><script>alert(1)</script>`
	app.Get("/", func(c *fiber.Ctx) error { return h.renderRedirectPage(c, hostile) })

	res, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	page := string(body)

	if strings.Contains(page, "<script>alert(1)") || strings.Contains(page, `x";`) {
		t.Fatalf("the address escaped its string:\n%s", page)
	}
	if strings.Count(page, "</script>") != 1 {
		t.Fatalf("want the page's own closing script tag only, got:\n%s", page)
	}
	// Angle brackets are written as JavaScript unicode escapes.
	escaped := strings.NewReplacer("<", `\`+"u003c", ">", `\`+"u003e").Replace("</script><script>alert(1)</script>")
	if !strings.Contains(page, `window.location.href = "https://issuer.example/oauth2/auth?state=x\";`+escaped+`";`) {
		t.Fatalf("the address is not written as an escaped string:\n%s", page)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
}
