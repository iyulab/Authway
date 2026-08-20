package invitation

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// stubEmailSender captures the last invitation email instead of delivering
// it. Migration 020 stopped storing the plaintext token (only its hash is
// persisted — see Invitation.TokenHash), so a test that needs to call
// Accept/Decline with the real token can no longer read it back from the
// database or the returned struct. The outbound link is the only place the
// plaintext still appears.
type stubEmailSender struct {
	lastLinkURL string
}

func (s *stubEmailSender) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	s.lastLinkURL = inviteURL
	return nil
}

// tokenFromLinkURL extracts the token query parameter an invitation email
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

// These tests run the invitation service against a real Postgres, because every
// defect they guard lives in the gap between the Go struct and the SQL schema —
// exactly what an AutoMigrate-from-struct harness cannot see. Two distinct bugs
// shipped in that gap:
//
//  1. The struct declared tenant_name/inviter_name/accepted_by columns that
//     migration 006 never created, so *every* INSERT failed.
//  2. inviter_id was NOT NULL REFERENCES users(id) while the admin-key path
//     attributed invitations to a hard-coded UUID with no users row, so an
//     empty instance could never produce its first user (creating a user needed
//     an invitation; an invitation needed a user).
//
// Gated on the same DSN as the migration tests; the schema is brought up by the
// real migrator, which is idempotent.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres invitation tests")
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

// newTestService wires the real service with no email sender — Create must
// succeed without any mail infrastructure, which is the situation a freshly
// provisioned instance is actually in.
func newTestService(t *testing.T, db *gorm.DB) (Service, *tenant.Service, user.Service) {
	t.Helper()
	tenantService := tenant.NewService(db)
	userService := user.NewService(db, zap.NewNop())
	svc := NewService(db, userService, tenantService, nil, zap.NewNop(), "http://localhost:3001")
	return svc, tenantService, userService
}

// newTestServiceWithSender is newTestService plus a stubEmailSender, for the
// tests that need to redeem an invitation and so need its plaintext token —
// which, since migration 020, exists nowhere but the outbound email.
func newTestServiceWithSender(t *testing.T, db *gorm.DB) (Service, *tenant.Service, user.Service, *stubEmailSender) {
	t.Helper()
	tenantService := tenant.NewService(db)
	userService := user.NewService(db, zap.NewNop())
	sender := &stubEmailSender{}
	svc := NewService(db, userService, tenantService, sender, zap.NewNop(), "http://localhost:3001")
	return svc, tenantService, userService, sender
}

// freshTenant creates an empty tenant, i.e. one with no users at all — the
// bootstrap situation. Slug is unique per run so repeated runs do not collide.
func freshTenant(t *testing.T, ts *tenant.Service) *tenant.Tenant {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{
		Name: "bootstrap-test-" + suffix,
		Slug: "bootstrap-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tn
}

// TestCreate_SystemActor_BootstrapsEmptyTenant is the regression guard for the
// bootstrap deadlock. It asserts what a fresh instance actually needs: an
// admin-key caller (nil inviter) can invite into a tenant that contains zero
// users.
func TestCreate_SystemActor_BootstrapsEmptyTenant(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, us := newTestService(t, db)
	tn := freshTenant(t, ts)

	// Precondition: the tenant really is empty. Without this the test could
	// pass for the wrong reason.
	_, total, err := us.GetByTenant(tn.ID, 10, 0)
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected a tenant with no users, got %d", total)
	}

	email := fmt.Sprintf("first-%s@example.com", uuid.New().String()[:8])
	inv, err := svc.Create(tn.ID, nil, &CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("system-actor invitation must succeed on an empty tenant, got: %v", err)
	}
	defer db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)

	if inv.InviterID != nil {
		t.Errorf("system-actor invitation must have a NULL inviter, got %v", *inv.InviterID)
	}
	if inv.InviterName != SystemInviterName {
		t.Errorf("InviterName = %q, want %q", inv.InviterName, SystemInviterName)
	}
	if inv.TenantName != tn.Name {
		t.Errorf("TenantName = %q, want %q", inv.TenantName, tn.Name)
	}
	if inv.Role != "member" {
		t.Errorf("Role = %q, want the default %q", inv.Role, "member")
	}

	// The NULL must have reached the column, not just the struct.
	var nullCount int64
	if err := db.Raw(
		`SELECT count(*) FROM invitations WHERE id = ? AND inviter_id IS NULL`, inv.ID,
	).Scan(&nullCount).Error; err != nil {
		t.Fatalf("verify null inviter: %v", err)
	}
	if nullCount != 1 {
		t.Errorf("inviter_id is not NULL in the database")
	}

	// Read-back must hydrate the derived fields too — they are not columns, so
	// a naive SELECT returns them empty.
	got, err := svc.GetByID(inv.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.InviterName != SystemInviterName || got.TenantName != tn.Name {
		t.Errorf("hydration lost on read: inviter=%q tenant=%q", got.InviterName, got.TenantName)
	}
}

// TestAccept_SystemActorInvitation_CreatesFirstUser closes the loop: the
// invitation issued above must actually be redeemable into the tenant's first
// user. Create succeeding is not enough — the deadlock is only broken if a user
// exists at the end.
func TestAccept_SystemActorInvitation_CreatesFirstUser(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, us, sender := newTestServiceWithSender(t, db)
	tn := freshTenant(t, ts)

	email := fmt.Sprintf("owner-%s@example.com", uuid.New().String()[:8])
	inv, err := svc.Create(tn.ID, nil, &CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)

	// Accept needs the raw token. Migration 020 stopped persisting it
	// (only its hash lives on the row), so it must come from the outbound
	// email — the same place the auth UI recovers it.
	token := tokenFromLinkURL(t, sender.lastLinkURL)
	u, err := svc.Accept(token, nil, "First Owner", "correct-horse-battery")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)

	if u.TenantID != tn.ID {
		t.Errorf("user landed in tenant %v, want %v", u.TenantID, tn.ID)
	}
	if u.Email != email {
		t.Errorf("email = %q, want %q", u.Email, email)
	}

	_, total, err := us.GetByTenant(tn.ID, 10, 0)
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if total != 1 {
		t.Errorf("tenant should now hold exactly its first user, got %d", total)
	}

	accepted, err := svc.GetByID(inv.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if accepted.Status != StatusAccepted {
		t.Errorf("status = %q, want %q", accepted.Status, StatusAccepted)
	}
	if accepted.AcceptedBy == nil || *accepted.AcceptedBy != u.ID {
		t.Errorf("accepted_by not recorded against the new user")
	}
}

// TestCreate_UserInviter_IsAttributed is the positive twin of the two
// system-actor tests: making the inviter optional must not have weakened the
// ordinary path where a signed-in member invites someone.
func TestCreate_UserInviter_IsAttributed(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, us := newTestService(t, db)
	tn := freshTenant(t, ts)

	inviter, err := us.Create(tn.ID, &user.CreateUserRequest{
		Email:    fmt.Sprintf("inviter-%s@example.com", uuid.New().String()[:8]),
		Password: "correct-horse-battery",
		Name:     "Real Inviter",
	})
	if err != nil {
		t.Fatalf("seed inviter: %v", err)
	}
	defer db.Exec(`DELETE FROM users WHERE id = ?`, inviter.ID)

	inv, err := svc.Create(tn.ID, &inviter.ID, &CreateInvitationRequest{
		Email: fmt.Sprintf("colleague-%s@example.com", uuid.New().String()[:8]),
	})
	if err != nil {
		t.Fatalf("a member inviting into their own tenant must succeed: %v", err)
	}
	defer db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)

	if inv.InviterID == nil || *inv.InviterID != inviter.ID {
		t.Errorf("inviter not recorded: %v", inv.InviterID)
	}
	if inv.InviterName != "Real Inviter" {
		t.Errorf("InviterName = %q, want the inviter's name", inv.InviterName)
	}

	// Hydration must resolve a real inviter on read, not fall back to "system".
	got, err := svc.GetByID(inv.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.InviterName != "Real Inviter" {
		t.Errorf("read-back InviterName = %q", got.InviterName)
	}
}

// TestCreate_UserInviter_RejectsCrossTenant guards the tenant isolation that the
// nil-inviter change made reachable: a real inviter must belong to the tenant
// they are inviting into, otherwise an id from another organization would be
// recorded as the inviter.
func TestCreate_UserInviter_RejectsCrossTenant(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, us := newTestService(t, db)
	tenantA := freshTenant(t, ts)
	tenantB := freshTenant(t, ts)

	outsider, err := us.Create(tenantB.ID, &user.CreateUserRequest{
		Email:    fmt.Sprintf("outsider-%s@example.com", uuid.New().String()[:8]),
		Password: "correct-horse-battery",
		Name:     "Outsider",
	})
	if err != nil {
		t.Fatalf("seed outsider: %v", err)
	}
	defer db.Exec(`DELETE FROM users WHERE id = ?`, outsider.ID)

	_, err = svc.Create(tenantA.ID, &outsider.ID, &CreateInvitationRequest{
		Email: fmt.Sprintf("victim-%s@example.com", uuid.New().String()[:8]),
	})
	if err == nil {
		t.Fatal("an inviter from another tenant must be rejected")
	}
}
