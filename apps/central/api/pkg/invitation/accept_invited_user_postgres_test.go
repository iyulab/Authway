package invitation

import (
	"testing"

	"authway/apps/central/api/pkg/user"
)

// An invitation is accepted for the user it was sent to, once, and only while
// it is pending — whatever order the acceptances arrive in.
func TestAcceptInvitedUser(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, users, sender := newTestServiceWithSender(t, db)
	tn := freshTenant(t, ts)

	invite := func(email string) *Invitation {
		t.Helper()
		inv, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: email})
		if err != nil {
			t.Fatalf("create invitation: %v", err)
		}
		_ = sender
		return inv
	}
	member := func(email string) *user.User {
		t.Helper()
		u, err := users.Create(tn.ID, &user.CreateUserRequest{Email: email, Name: "Member"})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u
	}

	t.Run("the user with the invited address closes it, once", func(t *testing.T) {
		email := "invited-" + tn.Slug + "@example.test"
		inv := invite(email)
		u := member(email)
		if err := svc.AcceptInvitedUser(inv.ID, u.ID); err != nil {
			t.Fatalf("accept: %v", err)
		}
		got, err := svc.GetByID(inv.ID)
		if err != nil || got.Status != StatusAccepted || got.AcceptedBy == nil || *got.AcceptedBy != u.ID {
			t.Fatalf("invitation after accept = %+v (%v), want accepted by the user", got, err)
		}
		if err := svc.AcceptInvitedUser(inv.ID, u.ID); err == nil {
			t.Fatal("an accepted invitation was accepted again")
		}
	})

	t.Run("a user with another address does not close it", func(t *testing.T) {
		inv := invite("someone-" + tn.Slug + "@example.test")
		other := member("other-" + tn.Slug + "@example.test")
		if err := svc.AcceptInvitedUser(inv.ID, other.ID); err == nil {
			t.Fatal("an invitation was accepted for a user with another address")
		}
		if got, _ := svc.GetByID(inv.ID); got == nil || got.Status != StatusPending {
			t.Fatalf("invitation after a refused accept = %+v, want pending", got)
		}
	})

	t.Run("the address is compared without case", func(t *testing.T) {
		inv := invite("Mixed-" + tn.Slug + "@Example.Test")
		u := member("mixed-" + tn.Slug + "@example.test")
		if err := svc.AcceptInvitedUser(inv.ID, u.ID); err != nil {
			t.Fatalf("accept: %v", err)
		}
	})

	t.Run("a revoked invitation cannot be closed after it was read", func(t *testing.T) {
		email := "revoked-" + tn.Slug + "@example.test"
		inv := invite(email)
		u := member(email)
		if err := svc.Revoke(inv.ID); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		if err := svc.AcceptInvitedUser(inv.ID, u.ID); err == nil {
			t.Fatal("a revoked invitation was accepted")
		}
	})

	t.Run("the conditional close refuses what is no longer pending", func(t *testing.T) {
		email := "race-" + tn.Slug + "@example.test"
		inv := invite(email)
		u := member(email)
		stale, err := svc.GetByID(inv.ID) // read while pending
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Revoke(inv.ID); err != nil {
			t.Fatal(err)
		}
		impl := svc.(*service)
		if err := impl.closeAccepted(stale, u.ID); err == nil {
			t.Fatal("closeAccepted closed an invitation revoked after the read")
		}
	})
}
