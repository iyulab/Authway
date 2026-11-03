package user

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
)

// Accounts that were linked through the four provider columns get identity
// rows: the migration copies them, an empty column links nothing, and running
// it again changes nothing.
func TestIdentities_AreCopiedFromTheProviderColumns(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	u, err := svc.Create(tn.ID, &CreateUserRequest{Email: fmt.Sprintf("copy-%s@example.com", uuid.NewString()[:8]), Name: "Copied"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	g, gh := "g-"+uuid.NewString(), "gh-"+uuid.NewString()
	if err := db.Exec(`UPDATE users SET google_id = ?, github_id = ?, microsoft_id = '' WHERE id = ?`, g, gh, u.ID).Error; err != nil {
		t.Fatal(err)
	}

	migration, err := os.ReadFile("../../internal/database/migrations/054_add_user_identities.sql")
	if err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 2; run++ {
		if err := db.Exec(string(migration)).Error; err != nil {
			t.Fatalf("migration, run %d: %v", run, err)
		}
		var got []LinkedIdentity
		db.Where("user_id = ?", u.ID).Order("provider").Find(&got)
		if len(got) != 2 || got[0].Provider != "github" || got[0].Subject != gh || got[1].Provider != "google" || got[1].Subject != g {
			t.Fatalf("run %d: identities = %+v, want github and google only (an empty column links nothing)", run, got)
		}
		if got[0].TenantID != tn.ID || got[0].EmailAtLink != u.Email {
			t.Errorf("run %d: tenant %v / address %q, want the account's", run, got[0].TenantID, got[0].EmailAtLink)
		}
	}
}

// A link is written to the column that is read and to an identity row beside
// it, a link to another account at the same provider replaces both, and
// taking the link away removes both.
func TestIdentities_AreWrittenBesideTheColumns(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	u, err := svc.Create(tn.ID, &CreateUserRequest{Email: fmt.Sprintf("beside-%s@example.com", uuid.NewString()[:8]), Name: "Beside", Password: "correct-horse-9"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	first, second := "g-"+uuid.NewString(), "g-"+uuid.NewString()

	state := func() (column *string, rows []LinkedIdentity) {
		db.Raw(`SELECT google_id FROM users WHERE id = ?`, u.ID).Scan(&column)
		db.Where("user_id = ?", u.ID).Find(&rows)
		return
	}
	if err := svc.LinkProvider(u.ID, "google", first); err != nil {
		t.Fatalf("LinkProvider: %v", err)
	}
	if err := svc.LinkProvider(u.ID, "google", first); err != nil {
		t.Errorf("linking again: %v", err)
	}
	column, rows := state()
	if column == nil || *column != first || len(rows) != 1 || rows[0].Subject != first || rows[0].Provider != "google" || rows[0].TenantID != tn.ID {
		t.Fatalf("after linking: column %v, rows %+v; want both at %q", column, rows, first)
	}

	if err := svc.LinkProvider(u.ID, "google", second); err != nil {
		t.Fatalf("LinkProvider to another account: %v", err)
	}
	if column, rows = state(); *column != second || len(rows) != 1 || rows[0].Subject != second {
		t.Fatalf("after replacing: column %v, rows %+v; want both at %q", *column, rows, second)
	}

	if err := svc.UnlinkProvider(u.ID, "google"); err != nil {
		t.Fatalf("UnlinkProvider: %v", err)
	}
	if column, rows = state(); column != nil || len(rows) != 0 {
		t.Errorf("after unlinking: column %v, rows %+v; want none", column, rows)
	}
	if _, err := svc.GetByProviderSubject(tn.ID, "google", second); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("the removed link still finds an account: %v", err)
	}
}
