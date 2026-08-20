package passwordless

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// stubEmailSender captures the last magic-link email instead of delivering
// it. Migration 019 stopped storing the plaintext token (only its hash is
// persisted — see MagicLink.TokenHash), so a raw DB read can no longer
// recover the token a test needs to call VerifyMagicLink/InspectMagicLink
// with. The outbound link is the only place the plaintext still appears.
type stubEmailSender struct {
	lastLinkURL string
}

func (s *stubEmailSender) SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error {
	s.lastLinkURL = linkURL
	return nil
}

// tokenFromLinkURL extracts the token query parameter a magic-link email
// points at, mirroring what the auth UI does when the link is opened.
func tokenFromLinkURL(t *testing.T, linkURL string) string {
	t.Helper()
	u, err := url.Parse(linkURL)
	if err != nil {
		t.Fatalf("parse link URL %q: %v", linkURL, err)
	}
	token := u.Query().Get("token")
	if token == "" {
		t.Fatalf("link URL %q carried no token", linkURL)
	}
	return token
}

// These tests guard the invitation-only policy on the magic-link path. The
// endpoint is public and unauthenticated, and before the gate existed it
// provisioned a user for any (email, tenant_id) pair a caller supplied — public
// self-registration in all but name, against a system that had declared
// self-registration removed.
//
// They run against real Postgres for the same reason the invitation tests do:
// the behaviour spans magic_link_tokens, invitations and users together.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres passwordless tests")
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

type fixture struct {
	db         *gorm.DB
	svc        Service
	invitation invitation.Service
	tenant     *tenant.Tenant
	sender     *stubEmailSender
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupPostgres(t)
	tenantService := tenant.NewService(db)
	userService := user.NewService(db, zap.NewNop())
	invService := invitation.NewService(db, userService, tenantService, nil, zap.NewNop(), "http://localhost:3001")

	suffix := uuid.New().String()[:8]
	tn, err := tenantService.CreateTenant(tenant.CreateTenantRequest{
		Name: "gate-test-" + suffix,
		Slug: "gate-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })

	sender := &stubEmailSender{}
	svc := NewService(db, userService, invService, sender, zap.NewNop(), "http://localhost:3001")
	return &fixture{db: db, svc: svc, invitation: invService, tenant: tn, sender: sender}
}

// lastToken returns the plaintext token from the most recently sent magic
// link. It must be read from the outbound email, not the database — see
// stubEmailSender.
func (f *fixture) lastToken(t *testing.T) string {
	t.Helper()
	return tokenFromLinkURL(t, f.sender.lastLinkURL)
}

func (f *fixture) linkCount(t *testing.T, email string) int64 {
	t.Helper()
	var n int64
	if err := f.db.Raw(
		`SELECT count(*) FROM magic_link_tokens WHERE tenant_id = ? AND email = ?`,
		f.tenant.ID, email,
	).Scan(&n).Error; err != nil {
		t.Fatalf("count links: %v", err)
	}
	return n
}

// TestSendMagicLink_UninvitedAddress_IssuesNoToken is the primary regression
// guard: an uninvited address must get no usable link, while the response stays
// indistinguishable from success so the endpoint cannot be used to enumerate
// who belongs to a tenant.
func TestSendMagicLink_UninvitedAddress_IssuesNoToken(t *testing.T) {
	f := newFixture(t)
	email := fmt.Sprintf("stranger-%s@example.com", uuid.New().String()[:8])

	resp, err := f.svc.SendMagicLink(f.tenant.ID, &SendMagicLinkRequest{Email: email}, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("response must not reveal that the address is uninvited, got error: %v", err)
	}
	if resp == nil || resp.Message == "" {
		t.Fatal("expected the same success-shaped response as an invited address")
	}
	if n := f.linkCount(t, email); n != 0 {
		t.Errorf("no magic link may be issued for an uninvited address, found %d", n)
	}

	var users int64
	f.db.Raw(`SELECT count(*) FROM users WHERE tenant_id = ? AND email = ?`, f.tenant.ID, email).Scan(&users)
	if users != 0 {
		t.Errorf("no user may be provisioned for an uninvited address, found %d", users)
	}
}

// TestMagicLink_InvitedAddress_ProvisionsUser confirms the gate is a gate and
// not a wall: an invited address still onboards end to end.
func TestMagicLink_InvitedAddress_ProvisionsUser(t *testing.T) {
	f := newFixture(t)
	email := fmt.Sprintf("invited-%s@example.com", uuid.New().String()[:8])

	inv, err := f.invitation.Create(f.tenant.ID, nil, &invitation.CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	defer f.db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)

	if _, err := f.svc.SendMagicLink(f.tenant.ID, &SendMagicLinkRequest{Email: email}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n := f.linkCount(t, email); n != 1 {
		t.Fatalf("expected exactly one magic link, found %d", n)
	}

	token := f.lastToken(t)

	_, u, err := f.svc.VerifyMagicLink(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	defer f.db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
	if u.Email != email || u.TenantID != f.tenant.ID {
		t.Errorf("provisioned the wrong user: %s in %v", u.Email, u.TenantID)
	}
}

// TestVerifyMagicLink_RevokedInvitation_Denies covers the window between send
// and verify. Holding a link is not proof of eligibility — the invitation can
// be revoked in the meantime, and verify must re-check rather than trust the
// decision made at send time.
func TestVerifyMagicLink_RevokedInvitation_Denies(t *testing.T) {
	f := newFixture(t)
	email := fmt.Sprintf("revoked-%s@example.com", uuid.New().String()[:8])

	inv, err := f.invitation.Create(f.tenant.ID, nil, &invitation.CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	defer f.db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)

	if _, err := f.svc.SendMagicLink(f.tenant.ID, &SendMagicLinkRequest{Email: email}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("send: %v", err)
	}
	token := f.lastToken(t)

	if err := f.invitation.Revoke(inv.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if _, _, err := f.svc.VerifyMagicLink(token); err == nil {
		f.db.Exec(`DELETE FROM users WHERE tenant_id = ? AND email = ?`, f.tenant.ID, email)
		t.Fatal("a link whose invitation was revoked must not provision a user")
	}
}

// TestSendMagicLink_ExistingUser_IsUnaffected pins that the gate applies to
// provisioning only. An existing member logging in never needs an invitation.
func TestSendMagicLink_ExistingUser_IsUnaffected(t *testing.T) {
	f := newFixture(t)
	userService := user.NewService(f.db, zap.NewNop())
	email := fmt.Sprintf("member-%s@example.com", uuid.New().String()[:8])

	u, err := userService.Create(f.tenant.ID, &user.CreateUserRequest{
		Email: email, Password: "correct-horse-battery", Name: "Member",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	defer f.db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)

	if _, err := f.svc.SendMagicLink(f.tenant.ID, &SendMagicLinkRequest{Email: email}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n := f.linkCount(t, email); n != 1 {
		t.Fatalf("an existing member must still receive a login link, found %d", n)
	}

	var tokenType string
	f.db.Raw(`SELECT token_type FROM magic_link_tokens WHERE tenant_id = ? AND email = ?`, f.tenant.ID, email).Scan(&tokenType)
	if tokenType != string(TokenTypeLogin) {
		t.Errorf("token_type = %q, want %q", tokenType, TokenTypeLogin)
	}
}

// TestInspectMagicLink_DoesNotConsume guards the status endpoint's contract.
// It used to call VerifyMagicLink, so merely *checking* a link marked it used
// and provisioned the user — an email scanner that prefetches links consumed
// them before the recipient clicked.
func TestInspectMagicLink_DoesNotConsume(t *testing.T) {
	f := newFixture(t)
	email := fmt.Sprintf("inspect-%s@example.com", uuid.New().String()[:8])

	inv, err := f.invitation.Create(f.tenant.ID, nil, &invitation.CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	defer f.db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)
	if _, err := f.svc.SendMagicLink(f.tenant.ID, &SendMagicLinkRequest{Email: email}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("send: %v", err)
	}
	token := f.lastToken(t)

	// Inspect repeatedly — a read must be repeatable.
	for i := range 3 {
		if _, err := f.svc.InspectMagicLink(token); err != nil {
			t.Fatalf("inspect #%d: %v", i+1, err)
		}
	}

	var used *string
	f.db.Raw(`SELECT used_at::text FROM magic_link_tokens WHERE token_hash = ?`, tokenhash.Hash(token)).Scan(&used)
	if used != nil {
		t.Errorf("inspecting marked the link used (used_at=%v)", *used)
	}
	var users int64
	f.db.Raw(`SELECT count(*) FROM users WHERE tenant_id = ? AND email = ?`, f.tenant.ID, email).Scan(&users)
	if users != 0 {
		t.Errorf("inspecting provisioned a user (%d)", users)
	}

	// And the link must still be redeemable afterwards.
	_, u, err := f.svc.VerifyMagicLink(token)
	if err != nil {
		t.Fatalf("verify after inspect: %v", err)
	}
	f.db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
}

// TestVerifyMagicLink_IsSingleUse guards the token claim. Verification used to
// read the row, check "not used", then write — so two requests could both pass
// the check and both redeem one link. The claim is now a single conditional
// UPDATE, which exactly one caller can win.
func TestVerifyMagicLink_IsSingleUse(t *testing.T) {
	f := newFixture(t)
	email := fmt.Sprintf("single-%s@example.com", uuid.New().String()[:8])

	inv, err := f.invitation.Create(f.tenant.ID, nil, &invitation.CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	defer f.db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)
	if _, err := f.svc.SendMagicLink(f.tenant.ID, &SendMagicLinkRequest{Email: email}, "127.0.0.1", "test"); err != nil {
		t.Fatalf("send: %v", err)
	}
	token := f.lastToken(t)

	_, u, err := f.svc.VerifyMagicLink(token)
	if err != nil {
		t.Fatalf("first verify: %v", err)
	}
	defer f.db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)

	if _, _, err := f.svc.VerifyMagicLink(token); err == nil {
		t.Error("a magic link must be redeemable only once")
	}
}

// TestMayProvision_FailsClosed pins the wiring contract: a service built without
// an invitation gate denies provisioning instead of falling back to "no policy".
func TestMayProvision_FailsClosed(t *testing.T) {
	f := newFixture(t)
	userService := user.NewService(f.db, zap.NewNop())
	ungated := NewService(f.db, userService, nil, nil, zap.NewNop(), "http://localhost:3001").(*service)

	if ungated.mayProvision(f.tenant.ID, "anyone@example.com") {
		t.Error("a missing invitation gate must deny provisioning, not allow it")
	}
}
