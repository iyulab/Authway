package delegation

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
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
)

// setupPostgres mirrors the same-named helper in pkg/terms: delegations
// live in Postgres and are read joined with the clients and users tables.
func setupPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live Postgres delegation tests")
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
	tenantID, userID, otherUserID, clientRow uuid.UUID
	clientID                                 string
}

func fixture(t *testing.T, db *gorm.DB) world {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := tenant.NewService(db).CreateTenant(tenant.CreateTenantRequest{Name: "delegation-test-" + suffix, Slug: "delegation-test-" + suffix})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	users := user.NewService(db, zap.NewNop())
	var ids [2]uuid.UUID
	for i := range ids {
		u, err := users.Create(tn.ID, &user.CreateUserRequest{Email: fmt.Sprintf("delegation-%s-%d@example.com", suffix, i), Name: "Delegation Test User"})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		ids[i] = u.ID
	}
	w := world{tenantID: tn.ID, userID: ids[0], otherUserID: ids[1], clientRow: uuid.New(), clientID: "delegation-test-" + suffix}
	if err := db.Exec(`INSERT INTO clients (id, tenant_id, client_id, client_secret, name, grant_types, scopes, created_at, updated_at)
		VALUES (?, ?, ?, '', 'Delegation Test Client', '{authorization_code}', '{openid}', now(), now())`,
		w.clientRow, tn.ID, w.clientID).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM delegation_grants WHERE tenant_id = ?`, tn.ID)
		db.Exec(`DELETE FROM clients WHERE id = ?`, w.clientRow)
		db.Exec(`DELETE FROM users WHERE id IN (?, ?)`, ids[0], ids[1])
		db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID)
	})
	return w
}

// clock advances a second per call, so delegations granted in one test have
// distinct times.
func clock() func() time.Time {
	t := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	return func() time.Time { t = t.Add(time.Second); return t }
}

func TestGrant_KeepsOnlyTheHashAndListsTheDelegation(t *testing.T) {
	db := setupPostgres(t)
	w := fixture(t, db)
	s := &service{db: db, now: clock()}
	ctx := context.Background()

	handle, g, err := s.Grant(ctx, w.tenantID, w.userID, w.clientRow, []string{"openid", "orders.read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(handle) < 43 || g.HandleHash != tokenhash.Hash(handle) {
		t.Fatalf("handle %q: want 256 bits, stored as its hash only", handle)
	}
	var stored int64
	db.Raw(`SELECT count(*) FROM delegation_grants WHERE handle_hash = ?`, handle).Scan(&stored)
	if stored != 0 {
		t.Fatal("the handle itself is stored")
	}

	list, err := s.List(ctx, w.tenantID, Filter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v; want the one delegation", list, err)
	}
	v := list[0]
	if v.ID != g.ID || v.UserID != w.userID || v.ClientID != w.clientID || v.ClientName != "Delegation Test Client" || len(v.Scopes) != 2 {
		t.Fatalf("view = %+v", v)
	}
	mine, _ := s.ListForUser(ctx, w.userID)
	theirs, _ := s.ListForUser(ctx, w.otherUserID)
	if len(mine) != 1 || len(theirs) != 0 {
		t.Fatalf("a user's list holds their own delegations only: mine %d, theirs %d", len(mine), len(theirs))
	}
	for _, f := range []Filter{{UserID: w.otherUserID}, {ClientID: "another-client"}} {
		if got, _ := s.List(ctx, w.tenantID, f); len(got) != 0 {
			t.Errorf("filter %+v: %d delegations, want none", f, len(got))
		}
	}
}

func TestRevoke_EndsTheDelegationOnce(t *testing.T) {
	db := setupPostgres(t)
	w := fixture(t, db)
	s := &service{db: db, now: clock()}
	ctx := context.Background()

	_, g, err := s.Grant(ctx, w.tenantID, w.userID, w.clientRow, []string{"openid"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(ctx, g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("find after revoke: %v, want ErrNotFound", err)
	}
	if err := s.Revoke(ctx, g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v, want ErrNotFound", err)
	}
	var revokedAt *time.Time
	db.Raw(`SELECT revoked_at FROM delegation_grants WHERE id = ?`, g.ID).Scan(&revokedAt)
	if revokedAt == nil {
		t.Fatal("a revoked delegation keeps its row, with revoked_at")
	}
}

func TestDelegationsOfARemovedUserOrClientAreNotInForce(t *testing.T) {
	db := setupPostgres(t)
	ctx := context.Background()
	for _, removed := range []string{"users", "clients"} {
		t.Run(removed, func(t *testing.T) {
			w := fixture(t, db)
			s := &service{db: db, now: clock()}
			_, g, err := s.Grant(ctx, w.tenantID, w.userID, w.clientRow, []string{"openid"})
			if err != nil {
				t.Fatal(err)
			}
			id := w.userID
			if removed == "clients" {
				id = w.clientRow
			}
			db.Exec(`UPDATE `+removed+` SET deleted_at = now() WHERE id = ?`, id)
			if _, err := s.Find(ctx, g.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("find: %v, want ErrNotFound once the %s row is deleted", err, removed)
			}
			if list, _ := s.List(ctx, w.tenantID, Filter{}); len(list) != 0 {
				t.Fatalf("list holds %d delegations of a deleted %s row", len(list), removed)
			}
		})
	}
}

func TestGrant_KeepsAtMostMaxPerClientInForce(t *testing.T) {
	db := setupPostgres(t)
	w := fixture(t, db)
	s := &service{db: db, now: clock()}
	ctx := context.Background()

	var first uuid.UUID
	for i := 0; i < MaxPerClient+2; i++ {
		_, g, err := s.Grant(ctx, w.tenantID, w.userID, w.clientRow, []string{"openid"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = g.ID
		}
	}
	list, _ := s.ListForUser(ctx, w.userID)
	if len(list) != MaxPerClient {
		t.Fatalf("%d in force, want %d", len(list), MaxPerClient)
	}
	if _, err := s.Find(ctx, first); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the oldest is still in force: %v", err)
	}
}
