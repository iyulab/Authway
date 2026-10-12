package social

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
)

// With the app registration set up to state that the address's domain owner
// verified it, a Microsoft sign-in links the account that has the address.
func TestMicrosoftSignIn_VerifiedAddressLinksTheAccount(t *testing.T) {
	tenantID := uuid.New()
	accounts := newFakeAccounts()
	owner := accounts.add(tenantID, victimEmail)

	svc := NewMicrosoftService(&config.MicrosoftOAuthConfig{ClientID: "ms-app", TenantID: "common"},
		accounts, &stubGate{}, oneTenantClients{tenantID: tenantID}, zap.NewNop())
	svc.httpClient = providerResponses{
		"/oauth2/v2.0/token":     `{"access_token":"at","id_token":"` + unsignedIDToken(t, map[string]any{"email": victimEmail, "xms_edov": true}) + `"}`,
		"graph.microsoft.com/v1": `{"id":"owner-object-id","mail":"` + victimEmail + `"}`,
	}.client()

	got, err := svc.HandleCallbackForClient(context.Background(), "code", "state", "authway_client")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != owner.ID || linkedSubject(owner, "microsoft") != "owner-object-id" || !owner.EmailVerified {
		t.Errorf("got %s linked=%q verified=%v, want the owner linked and verified", got.ID, linkedSubject(owner, "microsoft"), owner.EmailVerified)
	}
}

// Google says whether it verified the address; when it has not, the address
// finds no account.
func TestGoogleSignIn_UnverifiedAddressDoesNotReachAnExistingAccount(t *testing.T) {
	tenantID := uuid.New()
	accounts := newFakeAccounts()
	accounts.add(tenantID, victimEmail)

	svc := NewGoogleService(&config.GoogleOAuthConfig{ClientID: "g-app"}, accounts, &stubGate{invited: true}, oneTenantClients{tenantID: tenantID}, zap.NewNop())
	svc.httpClient = providerResponses{
		"oauth2.googleapis.com/token": `{"access_token":"at"}`,
		"oauth2/v2/userinfo":          `{"id":"g-1","email":"` + victimEmail + `","verified_email":false}`,
	}.client()

	if _, err := svc.HandleCallbackForClient(context.Background(), "code", "state", "authway_client"); !errors.Is(err, ErrEmailNotVerified) {
		t.Errorf("error = %v, want ErrEmailNotVerified", err)
	}
	if accounts.linked != 0 {
		t.Error("an account was linked on an unverified address")
	}
}

// The address a GitHub profile shows is the person's choice of what to
// display. Only the account's verified addresses may find an account.
func TestGitHubSignIn_ProfileAddressAloneDoesNotReachAnExistingAccount(t *testing.T) {
	tenantID := uuid.New()
	accounts := newFakeAccounts()
	victim := accounts.add(tenantID, victimEmail)

	svc := NewGitHubService(&config.GitHubOAuthConfig{ClientID: "gh-app"}, accounts, &stubGate{invited: true}, oneTenantClients{tenantID: tenantID}, zap.NewNop())
	svc.httpClient = providerResponses{
		"login/oauth/access_token":   `{"access_token":"at"}`,
		"api.github.com/user/emails": `[{"email":"mallory@example.net","primary":true,"verified":true}]`,
		"api.github.com/user":        `{"id":7,"login":"mallory","email":"` + victimEmail + `"}`,
	}.client()

	got, err := svc.HandleCallbackForClient(context.Background(), "code", "state", "authway_client")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == victim.ID || got.Email != "mallory@example.net" {
		t.Errorf("signed in as %s <%s>, want a separate account on the verified address", got.ID, got.Email)
	}
}

func TestAppleIDToken_ReadsWhetherTheAddressIsVerified(t *testing.T) {
	svc := &AppleService{}
	for claim, want := range map[any]bool{true: true, "true": true, false: false, "false": false} {
		info, err := svc.parseIDToken(unsignedIDToken(t, map[string]any{"sub": "a-1", "email": victimEmail, "email_verified": claim}))
		if err != nil {
			t.Fatal(err)
		}
		if info.EmailVerified != want || info.Sub != "a-1" {
			t.Errorf("email_verified %#v -> %v, want %v", claim, info.EmailVerified, want)
		}
	}
	info, err := svc.parseIDToken(unsignedIDToken(t, map[string]any{"sub": "a-1", "email": victimEmail}))
	if err != nil || info.EmailVerified {
		t.Errorf("no email_verified claim -> %v, %v; want unverified", info, err)
	}
}

func TestResolveAccount(t *testing.T) {
	tenantID := uuid.New()
	verified := Identity{Provider: "google", Subject: "g-1", Email: victimEmail, EmailVerified: true, Name: "Vic"}

	t.Run("a linked account is found by the link, whatever address the provider reports", func(t *testing.T) {
		accounts := newFakeAccounts()
		owner := accounts.add(tenantID, victimEmail)
		setLinkedSubject(owner, "google", "g-1")
		other := accounts.add(tenantID, "moved@example.com")

		got, err := resolveAccount(accounts, &stubGate{}, zap.NewNop(), tenantID,
			Identity{Provider: "google", Subject: "g-1", Email: other.Email, EmailVerified: false})
		if err != nil || got.ID != owner.ID {
			t.Errorf("got %v, %v; want the linked account", got, err)
		}
	})

	t.Run("a link in another tenant does not count", func(t *testing.T) {
		accounts := newFakeAccounts()
		elsewhere := accounts.add(uuid.New(), victimEmail)
		setLinkedSubject(elsewhere, "google", "g-1")

		_, err := resolveAccount(accounts, &stubGate{invited: false}, zap.NewNop(), tenantID, verified)
		if !errors.Is(err, ErrNotInvited) {
			t.Errorf("error = %v, want ErrNotInvited (no account in this tenant)", err)
		}
	})

	t.Run("an address already linked to another provider account is refused", func(t *testing.T) {
		accounts := newFakeAccounts()
		owner := accounts.add(tenantID, victimEmail)
		setLinkedSubject(owner, "google", "g-original")

		_, err := resolveAccount(accounts, &stubGate{invited: true}, zap.NewNop(), tenantID, verified)
		if !errors.Is(err, ErrLinkedToAnother) || linkedSubject(owner, "google") != "g-original" {
			t.Errorf("error = %v, link = %q; want ErrLinkedToAnother and the link unchanged", err, linkedSubject(owner, "google"))
		}
	})

	t.Run("an invited address gets a linked, verified account", func(t *testing.T) {
		accounts := newFakeAccounts()
		got, err := resolveAccount(accounts, &stubGate{invited: true}, zap.NewNop(), tenantID, verified)
		if err != nil {
			t.Fatal(err)
		}
		if linkedSubject(got, "google") != "g-1" || !got.EmailVerified || got.Name == nil || *got.Name != "Vic" {
			t.Errorf("created %+v, want it linked, verified and named", got)
		}
	})

	t.Run("an uninvited or unverified address creates nothing", func(t *testing.T) {
		for _, tc := range []struct {
			id   Identity
			gate *stubGate
			want error
		}{
			{verified, &stubGate{invited: false}, ErrNotInvited},
			{Identity{Provider: "google", Subject: "g-1", Email: victimEmail}, &stubGate{invited: true}, ErrEmailNotVerified},
		} {
			accounts := newFakeAccounts()
			if _, err := resolveAccount(accounts, tc.gate, zap.NewNop(), tenantID, tc.id); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
			if len(accounts.users) != 0 {
				t.Errorf("created %d account(s), want none", len(accounts.users))
			}
		}
	})

	t.Run("a failed lookup is a failure, not a new account", func(t *testing.T) {
		accounts := newFakeAccounts()
		accounts.lookupErr = errors.New("connection reset")
		if _, err := resolveAccount(accounts, &stubGate{invited: true}, zap.NewNop(), tenantID, verified); err == nil {
			t.Error("want an error when the link lookup fails")
		}
		if len(accounts.users) != 0 {
			t.Error("created an account although the lookup failed")
		}
	})
}
