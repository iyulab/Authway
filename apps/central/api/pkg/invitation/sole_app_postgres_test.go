package invitation

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// addClient registers a bare client row — what the invitation lookup reads of
// it — in the tenant.
func addClient(t *testing.T, db *gorm.DB, tenantID uuid.UUID, clientID, initiateLoginURI, clientURI string, active bool, systemKind *string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO clients (tenant_id, client_id, name, initiate_login_uri, client_uri, active, system_kind)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tenantID, clientID, "App "+clientID, initiateLoginURI, clientURI, active, systemKind,
	).Error
	if err != nil {
		t.Fatalf("add client %s: %v", clientID, err)
	}
}

// inviteAndLookUp invites an address into the tenant without naming an
// application, then reads the invitation the way the accept screen does.
func inviteAndLookUp(t *testing.T, svc Service, sender *stubEmailSender, tenantID uuid.UUID) *Invitation {
	t.Helper()
	if _, err := svc.Create(tenantID, Inviter{}, &CreateInvitationRequest{Email: "sole-" + uuid.NewString()[:8] + "@example.test"}); err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	got, err := svc.GetByToken(tokenFromLinkURL(t, sender.lastLinkURL))
	if err != nil {
		t.Fatalf("get by token: %v", err)
	}
	return got
}

// An invitation that names no application still leads somewhere when the
// tenant has exactly one application to lead to, and nowhere otherwise — the
// screen must not be handed a guess.
func TestGetByToken_AnInvitationWithoutAnAppLeadsToTheTenantsOnlyApp(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, _, sender := newTestServiceWithSender(t, db)

	t.Run("no application with an address: nowhere", func(t *testing.T) {
		tn := freshTenant(t, ts)
		addClient(t, db, tn.ID, "bare-"+uuid.NewString()[:8], "", "", true, nil)
		if got := inviteAndLookUp(t, svc, sender, tn.ID); got.App != nil {
			t.Fatalf("a client with no address was offered: %+v", got.App)
		}
	})

	t.Run("one application with an https address: that one", func(t *testing.T) {
		tn := freshTenant(t, ts)
		cid := "only-" + uuid.NewString()[:8]
		addClient(t, db, tn.ID, cid, "", "https://app.example.test/", true, nil)
		got := inviteAndLookUp(t, svc, sender, tn.ID)
		if got.App == nil || got.App.ClientID != cid || got.App.ClientURI != "https://app.example.test/" {
			t.Fatalf("expected the only application, got %+v", got.App)
		}
	})

	t.Run("two applications with an address: nowhere, not a guess", func(t *testing.T) {
		tn := freshTenant(t, ts)
		addClient(t, db, tn.ID, "a-"+uuid.NewString()[:8], "https://a.example.test/login", "", true, nil)
		addClient(t, db, tn.ID, "b-"+uuid.NewString()[:8], "", "https://b.example.test/", true, nil)
		if got := inviteAndLookUp(t, svc, sender, tn.ID); got.App != nil {
			t.Fatalf("one of two applications was offered: %+v", got.App)
		}
	})

	t.Run("a disabled, deleted, plain-http or system client is not an application to lead to", func(t *testing.T) {
		tn := freshTenant(t, ts)
		system := "account"
		addClient(t, db, tn.ID, "off-"+uuid.NewString()[:8], "", "https://off.example.test/", false, nil)
		addClient(t, db, tn.ID, "plain-"+uuid.NewString()[:8], "", "http://plain.example.test/", true, nil)
		addClient(t, db, tn.ID, "sys-"+uuid.NewString()[:8], "", "https://sys.example.test/", true, &system)
		gone := "gone-" + uuid.NewString()[:8]
		addClient(t, db, tn.ID, gone, "", "https://gone.example.test/", true, nil)
		if err := db.Exec(`UPDATE clients SET deleted_at = now() WHERE client_id = ?`, gone).Error; err != nil {
			t.Fatal(err)
		}
		if got := inviteAndLookUp(t, svc, sender, tn.ID); got.App != nil {
			t.Fatalf("a client that is not one to lead to was offered: %+v", got.App)
		}
	})

	t.Run("another tenant's application is never offered", func(t *testing.T) {
		tn, other := freshTenant(t, ts), freshTenant(t, ts)
		addClient(t, db, other.ID, "other-"+uuid.NewString()[:8], "", "https://other.example.test/", true, nil)
		if got := inviteAndLookUp(t, svc, sender, tn.ID); got.App != nil {
			t.Fatalf("another tenant's application was offered: %+v", got.App)
		}
	})
}

// An invitation that names an application keeps it, whatever else the tenant has.
func TestGetByToken_AnInvitationsOwnAppWinsOverTheTenantsOnlyApp(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, _, sender := newTestServiceWithSender(t, db)
	tn := freshTenant(t, ts)
	named := "named-" + uuid.NewString()[:8]
	addClient(t, db, tn.ID, named, "", "https://named.example.test/", true, nil)

	if _, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: "named-" + uuid.NewString()[:8] + "@example.test", ClientID: named}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.GetByToken(tokenFromLinkURL(t, sender.lastLinkURL))
	if err != nil {
		t.Fatal(err)
	}
	if got.App == nil || got.App.ClientID != named {
		t.Fatalf("the named application was not kept: %+v", got.App)
	}
}
