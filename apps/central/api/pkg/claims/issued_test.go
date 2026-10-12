package claims

import (
	"testing"

	"github.com/google/uuid"
)

// The issuer's claims win over stored ones in both tokens, whichever stored
// claims there are.
func TestIssued_IssuerClaimsWinOverStoredOnes(t *testing.T) {
	name := "Ada"
	issued := Issued{TenantID: uuid.New(), Email: "ada@example.com", Name: &name, EmailVerified: false}
	stored := ClaimMap{
		"tenant_id": uuid.New().String(), "email": "mallory@example.com", "name": "Mallory",
		"email_verified": true, "auth_time": 4102444800, "sub": "someone-else", "_source": "pending",
		"plan": "pro",
	}

	access := issued.AccessToken(stored)
	if access["tenant_id"] != issued.TenantID.String() || access["email"] != issued.Email || access["name"] != &name {
		t.Fatalf("access token: issuer claims displaced: %+v", access)
	}
	for _, key := range []string{"email_verified", "auth_time", "sub", "_source"} {
		if _, ok := access[key]; ok {
			t.Fatalf("access token carries the stored %s: %+v", key, access)
		}
	}
	if access["plan"] != "pro" {
		t.Fatalf("access token lost the stored custom claim: %+v", access)
	}

	id := issued.IDToken(stored)
	if id["email_verified"] != false || id["tenant_id"] != issued.TenantID.String() || id["plan"] != "pro" {
		t.Fatalf("ID token: %+v", id)
	}
	if _, ok := id["auth_time"]; ok {
		t.Fatalf("ID token carries the stored auth_time: %+v", id)
	}
}

func TestIsReserved(t *testing.T) {
	for _, key := range []string{"sub", "tenant_id", "email_verified", "auth_time", "ext", "_anything"} {
		if !IsReserved(key) {
			t.Errorf("IsReserved(%q) = false", key)
		}
	}
	for _, key := range []string{"plan", "workspace_id", "role", "subscription"} {
		if IsReserved(key) {
			t.Errorf("IsReserved(%q) = true", key)
		}
	}
}
