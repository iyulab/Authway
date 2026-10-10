package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// refusingMailer refuses every message the way the background sender does
// when its queue is full.
type refusingMailer struct{ email.EmailService }

func (refusingMailer) SendVerificationEmail(string, string) error  { return email.ErrMailQueueFull }
func (refusingMailer) SendPasswordResetEmail(string, string) error { return email.ErrMailQueueFull }

func emailApp(h *EmailHandler) *fiber.App {
	app := fiber.New()
	h.RegisterRoutes(app.Group("/api/v1"))
	return app
}

// postEmail returns the status and body of a request to one of the email
// self-service routes.
func postEmail(t *testing.T, app *fiber.App, path, address string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(fmt.Sprintf(`{"email":%q}`, address)))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func messageOf(t *testing.T, body string) string {
	t.Helper()
	var out struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out.Message
}

// An address that is already verified gets the same answer as one with no
// account — a different message would tell a caller which addresses are
// registered.
func TestSendVerification_VerifiedAndUnknownAnswerAlike(t *testing.T) {
	verified := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "verified@example.com", EmailVerified: true}
	h := &EmailHandler{userSvc: newFakeUserService(verified), clientSvc: newFakeClientService(), validator: validator.New(), logger: zap.NewNop()}
	app := emailApp(h)

	knownStatus, knownBody := postEmail(t, app, "/api/v1/email/send-verification", verified.Email)
	unknownStatus, unknownBody := postEmail(t, app, "/api/v1/email/send-verification", "nobody@example.com")

	if knownStatus != 200 || unknownStatus != 200 {
		t.Fatalf("status verified=%d unknown=%d, want 200 for both", knownStatus, unknownStatus)
	}
	if messageOf(t, knownBody) != messageOf(t, unknownBody) {
		t.Errorf("verified address answered %q, unknown address %q — they must match", knownBody, unknownBody)
	}
}

func handlerPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres email handler tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// A request for an existing, unverified account whose mail the sender
// refuses answers exactly like a request for an address with no account.
func TestEmailRequests_RefusedMailAnswersLikeNoAccount(t *testing.T) {
	db := handlerPostgres(t)
	suffix := uuid.New().String()[:8]
	tn, err := tenant.NewService(db).CreateTenant(tenant.CreateTenantRequest{Name: "mail-answer-" + suffix, Slug: "mail-answer-" + suffix})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
	users := user.NewService(db, zap.NewNop())
	u, err := users.Create(tn.ID, &user.CreateUserRequest{Email: "member-" + suffix + "@example.com", Name: "Member"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	h := &EmailHandler{
		emailRepo: email.NewRepository(db), emailSvc: refusingMailer{}, userSvc: users,
		clientSvc: newFakeClientService(), validator: validator.New(), logger: zap.NewNop(),
	}
	app := emailApp(h)

	for _, path := range []string{"/api/v1/email/send-verification", "/api/v1/email/forgot-password"} {
		memberStatus, memberBody := postEmail(t, app, path, u.Email)
		nobodyStatus, nobodyBody := postEmail(t, app, path, "nobody-"+suffix+"@example.com")
		if memberStatus != 200 || nobodyStatus != 200 {
			t.Errorf("%s: status member=%d (%s) nobody=%d, want 200 for both", path, memberStatus, memberBody, nobodyStatus)
			continue
		}
		if messageOf(t, memberBody) != messageOf(t, nobodyBody) {
			t.Errorf("%s: member answered %q, nobody %q — they must match", path, memberBody, nobodyBody)
		}
	}
}
