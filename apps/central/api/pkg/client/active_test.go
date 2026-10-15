package client

import (
	"encoding/json"
	"strings"
	"testing"
)

// A client turned off is registered with the authorization server with no
// grants — an empty list, not a missing one, which the server would fill with
// its default — and turned on, with the grants it has.
func TestRegistration_OfAClientTurnedOffHasNoGrants(t *testing.T) {
	c := &Client{ClientID: "app", Active: false, GrantTypes: []string{"authorization_code", "refresh_token"}}
	raw, err := json.Marshal(c.hydraRegistration(""))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"grant_types":[]`) {
		t.Fatalf("registration of a client turned off: %s", raw)
	}

	c.Active = true
	if got := c.hydraRegistration("").GrantTypes; len(got) != 2 {
		t.Fatalf("registration of a client turned on has grants %v", got)
	}
}
