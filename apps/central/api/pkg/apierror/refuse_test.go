package apierror

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

func answer(t *testing.T, h fiber.Handler) (int, map[string]string, map[string]any) {
	t.Helper()
	app := fiber.New()
	app.Get("/", h)
	res, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("body is not JSON: %s", raw)
	}
	headers := map[string]string{}
	for k := range res.Header {
		headers[k] = res.Header.Get(k)
	}
	return res.StatusCode, headers, body
}

// The *way family's problem details: type about:blank, title the reason
// phrase, detail, and code as an extension member.
func TestRefuse_WritesProblemDetails(t *testing.T) {
	status, headers, body := answer(t, func(c *fiber.Ctx) error {
		return Refuse(c, fiber.StatusConflict, "tenant_slug_taken", "That slug is taken.")
	})
	if status != fiber.StatusConflict || headers["Content-Type"] != "application/problem+json" {
		t.Fatalf("status %d, Content-Type %q", status, headers["Content-Type"])
	}
	want := map[string]any{"type": "about:blank", "title": "Conflict", "status": float64(409), "detail": "That slug is taken.", "code": "tenant_slug_taken"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
	if _, old := body["error"]; old {
		t.Error("the previous error member is still written")
	}
}

func TestRefuseWith_ExtensionsCannotReplaceTheStandardMembers(t *testing.T) {
	_, _, body := answer(t, func(c *fiber.Ctx) error {
		return RefuseWith(c, fiber.StatusBadRequest, "invalid_request", "Bad.", fiber.Map{"field": "name", "status": "pending", "code": "other"})
	})
	if body["field"] != "name" || body["status"] != float64(400) || body["code"] != "invalid_request" {
		t.Fatalf("got %v", body)
	}
}

func TestRefuse_AddsTheHeadersTheStatusCallsFor(t *testing.T) {
	_, headers, _ := answer(t, func(c *fiber.Ctx) error {
		return Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in.")
	})
	if headers["Www-Authenticate"] != "Bearer" {
		t.Errorf("401 WWW-Authenticate = %q, want Bearer", headers["Www-Authenticate"])
	}
	_, headers, _ = answer(t, func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderWWWAuthenticate, `Bearer error="invalid_token"`)
		return Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Invalid token")
	})
	if headers["Www-Authenticate"] != `Bearer error="invalid_token"` {
		t.Errorf("a handler's own challenge was replaced: %q", headers["Www-Authenticate"])
	}
	_, headers, body := answer(t, func(c *fiber.Ctx) error {
		return RefuseWith(c, fiber.StatusTooManyRequests, "too_many_requests", "Later.", fiber.Map{"retry_after": 30})
	})
	if headers["Retry-After"] != "30" || body["retry_after"] != float64(30) {
		t.Errorf("429 Retry-After = %q, body %v", headers["Retry-After"], body)
	}
}

func TestInvalid_NamesTheMemberNotTheGoField(t *testing.T) {
	type request struct {
		ClientID string `validate:"required"`
		Email    string `validate:"required,email"`
	}
	err := validator.New().Struct(request{Email: "x"})
	_, _, body := answer(t, func(c *fiber.Ctx) error { return Invalid(c, err) })
	if body["field"] != "client_id" || body["detail"] != "client_id is required" || body["code"] != "invalid_request" {
		t.Fatalf("got %v", body)
	}
	if strings.Contains(body["detail"].(string), "request.") {
		t.Fatal("the validator's own text leaked")
	}
	for in, want := range map[string]string{"ClientID": "client_id", "RedirectURIs": "redirect_uris", "Email": "email", "PasswordMinLength": "password_min_length", "HTTPServer": "http_server", "IDs": "ids", "URIsList": "uris_list"} {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every refusal goes through Refuse. A handler that writes {"error": …} itself
// brings back the previous shape; the only bodies allowed an "error" member are
// the ones whose standards define it (OAuth 2.0 error responses).
func TestNoHandlerWritesTheErrorMemberItself(t *testing.T) {
	allowed := map[string]string{
		"internal/handler/introspection.go": "RFC 7662 / RFC 6749 5.2 error response",
		"internal/hydra/client.go":          "the authorization server's own OAuth error response",
		"internal/handler/social.go":        "an audit log detail, not a response",
	}
	root := filepath.Join("..", "..")
	member := regexp.MustCompile(`"error"\s*:\s*[^{\s]`)
	// A refusal built from an error's own text leaks what produced it — a
	// validator names Go struct fields; use apierror.Invalid or a reviewed
	// message (apierror.Message) instead.
	leak := regexp.MustCompile(`NewError\([^)]*err\.Error\(\)`)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}
		src, _ := os.ReadFile(path)
		for i, line := range strings.Split(string(src), "\n") {
			if member.MatchString(line) && !strings.Contains(line, "url.Values") {
				t.Errorf("%s:%d writes an error member itself; use apierror.Refuse: %s", rel, i+1, strings.TrimSpace(line))
			}
			if leak.MatchString(line) {
				t.Errorf("%s:%d refuses with an error's own text: %s", rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
