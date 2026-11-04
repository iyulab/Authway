package invitation

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// localeSpy is the part of Service the language of an invitation email passes
// through: it records the language each call asked for. Every other method
// would panic, which is the point — nothing else may be reached.
type localeSpy struct {
	Service
	created *CreateInvitationRequest
	resent  string
	resends int
}

func (s *localeSpy) Create(_ uuid.UUID, _ Inviter, req *CreateInvitationRequest) (*Invitation, error) {
	copied := *req
	s.created = &copied
	return &Invitation{}, nil
}

func (s *localeSpy) GetByID(uuid.UUID) (*Invitation, error) { return nil, ErrNotFound }

func (s *localeSpy) Resend(_ uuid.UUID, locale string) error {
	s.resent = locale
	s.resends++
	return nil
}

func localeApp(spy *localeSpy) *fiber.App {
	h := NewHandler(spy, zap.NewNop())
	app := fiber.New()
	// The administration key has no user behind it; the authentication
	// middleware has named the tenant the call acts on.
	tenant := uuid.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("is_admin_console", true)
		c.Locals(tenantscope.LocalKey, tenant)
		return c.Next()
	})
	app.Post("/api/v1/invitations", h.CreateInvitation)
	app.Post("/api/v1/invitations/:id/resend", h.ResendInvitation)
	return app
}

func call(t *testing.T, app *fiber.App, path, body, acceptLanguage string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

// The one who invites knows the language of the one invited; a server calling
// for an application sends no Accept-Language. The call can name the language.
func TestCreateInvitation_TheCallNamesTheLanguageOfTheEmail(t *testing.T) {
	for _, c := range []struct {
		name, body, header, want string
	}{
		{"named, and no header", `{"email":"a@example.com","locale":"ko"}`, "", "ko"},
		{"a regional tag", `{"email":"a@example.com","locale":"ko-KR"}`, "", "ko"},
		{"named beats the header", `{"email":"a@example.com","locale":"en"}`, "ko-KR,ko;q=0.9", "en"},
		{"not named: the header", `{"email":"a@example.com"}`, "ko-KR,ko;q=0.9", "ko"},
		{"an empty name is not a name", `{"email":"a@example.com","locale":""}`, "ko", "ko"},
		{"not named, no header: English", `{"email":"a@example.com"}`, "", "en"},
		{"not named, a header with nothing to write in: English", `{"email":"a@example.com"}`, "fr-FR,fr;q=0.9", "en"},
	} {
		t.Run(c.name, func(t *testing.T) {
			spy := &localeSpy{}
			status, body := call(t, localeApp(spy), "/api/v1/invitations", c.body, c.header)
			if status != http.StatusCreated {
				t.Fatalf("status %d (%s), want 201", status, body)
			}
			if spy.created == nil || spy.created.Locale != c.want {
				t.Errorf("the email is written in %+v, want %q", spy.created, c.want)
			}
		})
	}
}

// A language Authway has no strings for is refused: the one who asked is told,
// instead of the invited being written to in a language nobody chose.
func TestCreateInvitation_ALanguageWithNoStringsIsRefused(t *testing.T) {
	for _, locale := range []string{"fr", "fr-FR", "*", "ko, en", "ko;q=0.5"} {
		spy := &localeSpy{}
		status, body := call(t, localeApp(spy), "/api/v1/invitations",
			`{"email":"a@example.com","locale":"`+locale+`"}`, "ko")
		if status != http.StatusBadRequest {
			t.Errorf("locale %q: status %d, want 400", locale, status)
		}
		if !strings.Contains(body, `"invalid_request"`) || !strings.Contains(body, `"locale"`) {
			t.Errorf("locale %q: answer %s does not name the code and the field", locale, body)
		}
		if spy.created != nil {
			t.Errorf("locale %q: an invitation was created anyway", locale)
		}
	}
}

// A resend carries the same choice, and its body stays optional.
func TestResendInvitation_TheCallMayNameTheLanguage(t *testing.T) {
	path := "/api/v1/invitations/" + uuid.NewString() + "/resend"
	for _, c := range []struct {
		name, body, header, want string
	}{
		{"no body, no header", "", "", "en"},
		{"no body: the header", "", "ko", "ko"},
		{"named", `{"locale":"ko"}`, "", "ko"},
		{"named beats the header", `{"locale":"en"}`, "ko", "en"},
		{"an empty object", `{}`, "ko", "ko"},
	} {
		t.Run(c.name, func(t *testing.T) {
			spy := &localeSpy{}
			status, body := call(t, localeApp(spy), path, c.body, c.header)
			if status != http.StatusOK {
				t.Fatalf("status %d (%s), want 200", status, body)
			}
			if spy.resent != c.want {
				t.Errorf("resent in %q, want %q", spy.resent, c.want)
			}
		})
	}

	spy := &localeSpy{}
	if status, _ := call(t, localeApp(spy), path, `{"locale":"fr"}`, "ko"); status != http.StatusBadRequest {
		t.Errorf("an unsupported language: status %d, want 400", status)
	}
	if spy.resends != 0 {
		t.Error("an invitation was resent in spite of the refused language")
	}
}
