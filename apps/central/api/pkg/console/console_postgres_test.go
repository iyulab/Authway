package console

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres console tests")
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

type world struct {
	system, other     uuid.UUID
	account, outsider uuid.UUID
}

func fixture(t *testing.T, db *gorm.DB) world {
	t.Helper()
	// A database the conformance suite ran against keeps its system tenant.
	// The test takes it over when no account of it holds a membership, and
	// gives it back afterwards.
	var previous []uuid.UUID
	db.Model(&tenant.Tenant{}).Where("is_system").Pluck("id", &previous)
	if len(previous) > 0 {
		var members int64
		db.Model(&Membership{}).Count(&members)
		if members > 0 {
			t.Skip("this database's system tenant has members; the tests would change who administers it")
		}
		db.Exec(`UPDATE tenants SET is_system = false WHERE is_system`)
	}
	suffix := uuid.New().String()[:8]
	tenants := tenant.NewService(db)
	users := user.NewService(db, zap.NewNop())
	var w world
	for i, id := range []*uuid.UUID{&w.system, &w.other} {
		tn, err := tenants.CreateTenant(tenant.CreateTenantRequest{Name: fmt.Sprintf("console-test-%s-%d", suffix, i), Slug: fmt.Sprintf("console-test-%s-%d", suffix, i)})
		if err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		*id = tn.ID
	}
	for i, pair := range []struct {
		tenant uuid.UUID
		id     *uuid.UUID
	}{{w.system, &w.account}, {w.other, &w.outsider}} {
		u, err := users.Create(pair.tenant, &user.CreateUserRequest{Email: fmt.Sprintf("console-%s-%d@example.com", suffix, i), Name: "Console Test"})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		*pair.id = u.ID
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM console_memberships WHERE user_id IN (?, ?)`, w.account, w.outsider)
		db.Exec(`UPDATE tenants SET is_system = false WHERE id IN (?, ?)`, w.system, w.other)
		for _, id := range previous {
			db.Exec(`UPDATE tenants SET is_system = true WHERE id = ?`, id)
		}
		db.Exec(`DELETE FROM users WHERE id IN (?, ?)`, w.account, w.outsider)
		db.Exec(`DELETE FROM tenants WHERE id IN (?, ?)`, w.system, w.other)
	})
	return w
}

func TestMemberships(t *testing.T) {
	db := setupPostgres(t)
	w := fixture(t, db)
	ctx := context.Background()
	s := NewService(db)

	if _, _, err := s.Put(ctx, w.account, &w.other, "owner"); !errors.Is(err, ErrNoSystemTenant) {
		t.Fatalf("Put before a system tenant = %v, want ErrNoSystemTenant", err)
	}
	if _, err := s.SetSystemTenant(ctx, w.system); err != nil {
		t.Fatalf("SetSystemTenant: %v", err)
	}
	if id, err := s.SystemTenantID(ctx); err != nil || id != w.system {
		t.Fatalf("SystemTenantID = %v, %v", id, err)
	}

	m, created, err := s.Put(ctx, w.account, &w.other, "owner")
	if err != nil || !created || m.Role != "owner" {
		t.Fatalf("Put owner = %+v, %v, %v", m, created, err)
	}
	again, created, err := s.Put(ctx, w.account, &w.other, "admin")
	if err != nil || created || again.ID != m.ID || again.Role != "admin" {
		t.Fatalf("Put admin over owner = %+v, %v, %v; want the same membership, role replaced", again, created, err)
	}
	if _, _, err := s.Put(ctx, w.account, nil, RoleOperator); err != nil {
		t.Fatalf("Put operator: %v", err)
	}

	for name, tc := range map[string]struct {
		user   uuid.UUID
		tenant *uuid.UUID
		role   string
		want   error
	}{
		"account of another tenant":  {w.outsider, &w.other, "admin", ErrNotConsoleAccount},
		"operator of one tenant":     {w.account, &w.other, RoleOperator, ErrInvalidRole},
		"owner of no tenant":         {w.account, nil, "owner", ErrInvalidRole},
		"unknown role":               {w.account, &w.other, "viewer", ErrInvalidRole},
		"unknown tenant":             {w.account, ptr(uuid.New()), "admin", ErrUnknownTenant},
		"owner of the system tenant": {w.account, &w.system, "owner", ErrSystemTenantMembership},
		"admin of the system tenant": {w.account, &w.system, "admin", ErrSystemTenantMembership},
	} {
		if _, _, err := s.Put(ctx, tc.user, tc.tenant, tc.role); !errors.Is(err, tc.want) {
			t.Errorf("%s: Put = %v, want %v", name, err, tc.want)
		}
	}

	ms, err := s.MembershipsOf(ctx, w.account)
	if err != nil || len(ms) != 2 {
		t.Fatalf("MembershipsOf = %d, %v; want 2", len(ms), err)
	}
	if s := ScopeOf(ms, nil); !s.IsPlatform() || s.IsAll() || s.Covers(w.other) {
		t.Fatal("an operator's scope: the platform, and inside no tenant — not even one it is a member of — without support access")
	}
	listed, err := s.List(ctx, []uuid.UUID{w.other}, false)
	if err != nil || len(listed) != 1 || listed[0].ID != m.ID {
		t.Fatalf("List(other) = %+v, %v", listed, err)
	}

	// The database holds the shape too: a second operator row for the
	// account, which a plain unique constraint would let through.
	if err := db.Exec(`INSERT INTO console_memberships (id, user_id, tenant_id, role, created_at) VALUES (?, ?, NULL, 'operator', ?)`,
		uuid.New(), w.account, time.Now()).Error; err == nil {
		t.Fatal("a second operator membership for one account was stored")
	}

	if _, err := s.SetSystemTenant(ctx, w.other); !errors.Is(err, ErrSystemTenantInUse) {
		t.Fatalf("replacing a system tenant whose accounts hold memberships = %v, want ErrSystemTenantInUse", err)
	}

	tenants := tenant.NewService(db)
	if err := tenants.DeleteTenant(w.system); !errors.Is(err, tenant.ErrSystemTenantProtected) {
		t.Fatalf("DeleteTenant(system) = %v, want ErrSystemTenantProtected", err)
	}
	off := false
	if _, err := tenants.UpdateTenant(w.system, tenant.UpdateTenantRequest{Active: &off}); !errors.Is(err, tenant.ErrSystemTenantProtected) {
		t.Fatalf("turning the system tenant off = %v, want ErrSystemTenantProtected", err)
	}

	for _, x := range ms {
		if err := s.Remove(ctx, x.ID); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}
	if err := s.Remove(ctx, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove twice = %v, want ErrNotFound", err)
	}
	if _, err := s.SetSystemTenant(ctx, w.other); err != nil {
		t.Fatalf("SetSystemTenant once no memberships are left: %v", err)
	}
	var systems int64
	db.Model(&tenant.Tenant{}).Where("is_system").Count(&systems)
	if systems != 1 {
		t.Fatalf("%d system tenants, want 1", systems)
	}
}

func ptr(id uuid.UUID) *uuid.UUID { return &id }

func TestSupportAccess(t *testing.T) {
	db := setupPostgres(t)
	w := fixture(t, db)
	ctx := context.Background()
	s := NewService(db)
	if _, err := s.SetSystemTenant(ctx, w.system); err != nil {
		t.Fatalf("SetSystemTenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM console_memberships WHERE user_id = ?`, w.account) })

	if _, err := s.StartSupport(ctx, w.account, w.other, "ticket 12"); !errors.Is(err, ErrNotOperator) {
		t.Fatalf("StartSupport without the operator role = %v, want ErrNotOperator", err)
	}
	if _, _, err := s.Put(ctx, w.account, nil, RoleOperator); err != nil {
		t.Fatalf("Put operator: %v", err)
	}
	for _, reason := range []string{"", "   ", string(make([]rune, SupportReasonMaxLength+1))} {
		if _, err := s.StartSupport(ctx, w.account, w.other, reason); !errors.Is(err, ErrSupportReason) {
			t.Fatalf("StartSupport with reason of %d runes = %v, want ErrSupportReason", len([]rune(reason)), err)
		}
	}
	if _, err := s.StartSupport(ctx, w.account, uuid.New(), "ticket 12"); !errors.Is(err, ErrUnknownTenant) {
		t.Fatalf("StartSupport to an unknown tenant = %v, want ErrUnknownTenant", err)
	}

	a, err := s.StartSupport(ctx, w.account, w.other, "  ticket 12  ")
	if err != nil {
		t.Fatalf("StartSupport: %v", err)
	}
	if a.Reason != "ticket 12" || a.ExpiresAt.Sub(a.StartedAt) != SupportAccessDuration {
		t.Fatalf("started = %+v", a)
	}
	open, err := s.OpenSupportTenants(ctx, w.account)
	if err != nil || len(open) != 1 || open[w.other] != a.ID {
		t.Fatalf("OpenSupportTenants = %v, %v; want [other]", open, err)
	}
	if s := ScopeOf([]Membership{{Role: RoleOperator}}, open); !s.Covers(w.other) || s.Covers(w.system) {
		t.Fatal("an operator with support access reaches that tenant only")
	}

	if _, err := s.EndSupport(ctx, w.outsider, a.ID); !errors.Is(err, ErrSupportNotFound) {
		t.Fatalf("EndSupport by another account = %v, want ErrSupportNotFound", err)
	}
	ended, err := s.EndSupport(ctx, w.account, a.ID)
	if err != nil || ended.EndedBy == nil || *ended.EndedBy != SupportEndedByOperator {
		t.Fatalf("EndSupport = %+v, %v", ended, err)
	}
	if _, err := s.EndSupport(ctx, w.account, a.ID); !errors.Is(err, ErrSupportNotFound) {
		t.Fatalf("ending it twice = %v, want ErrSupportNotFound", err)
	}
	if open, _ := s.OpenSupportTenants(ctx, w.account); len(open) != 0 {
		t.Fatalf("after ending: open = %v", open)
	}

	// Expiry: an entry past its hour is not open, and the daily close records
	// its end once.
	b, err := s.StartSupport(ctx, w.account, w.other, "ticket 13")
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE console_support_access SET expires_at = now() - interval '1 minute' WHERE id = ?`, b.ID)
	if open, _ := s.OpenSupportTenants(ctx, w.account); len(open) != 0 {
		t.Fatalf("an expired entry is open: %v", open)
	}
	closed, err := s.CloseExpiredSupport(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var mine []SupportAccess
	for _, c := range closed {
		if c.UserID == w.account {
			mine = append(mine, c)
		}
	}
	if len(mine) != 1 || mine[0].ID != b.ID || mine[0].TenantID != w.other || mine[0].EndedBy == nil || *mine[0].EndedBy != SupportEndedByExpiry {
		t.Fatalf("CloseExpiredSupport = %+v", mine)
	}
	if again, _ := s.CloseExpiredSupport(ctx, time.Now()); len(again) != 0 {
		for _, c := range again {
			if c.ID == b.ID {
				t.Fatal("an expired entry was closed twice")
			}
		}
	}
}
