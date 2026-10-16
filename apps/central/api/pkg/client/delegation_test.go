package client

import (
	"slices"
	"testing"
)

func TestValidateDelegation(t *testing.T) {
	cases := []struct {
		name   string
		public bool
		grants []string
		scopes []string
		code   string
	}{
		{"a confidential client that exchanges and asks for delegation", false, []string{"authorization_code", TokenExchangeGrant}, []string{"openid", DelegationScope}, ""},
		{"a confidential client that exchanges without asking", false, []string{"authorization_code", TokenExchangeGrant}, []string{"openid"}, ""},
		{"a client that neither exchanges nor asks", true, []string{"authorization_code"}, []string{"openid"}, ""},
		{"a public client that exchanges", true, []string{"authorization_code", TokenExchangeGrant}, []string{"openid"}, "public_client_with_token_exchange"},
		{"asking for delegation without the exchange grant", false, []string{"authorization_code"}, []string{"openid", DelegationScope}, "delegation_without_token_exchange"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateDelegation(tc.public, tc.grants, tc.scopes)
			switch {
			case tc.code == "" && got != nil:
				t.Fatalf("refused with %s", got.Code)
			case tc.code != "" && (got == nil || got.Code != tc.code):
				t.Fatalf("got %v, want %s", got, tc.code)
			}
		})
	}
	if validateGrantTypes([]string{TokenExchangeGrant}) != nil {
		t.Fatal("token exchange is a grant this provider offers")
	}
}

// The authorization server does not know token exchange by that name: it is
// answered by Authway, so it never reaches the client's registration there.
func TestGrantsInForceLeaveTokenExchangeToAuthway(t *testing.T) {
	c := &Client{Active: true, GrantTypes: []string{"authorization_code", TokenExchangeGrant, "refresh_token"}}
	if got := c.grantsInForce(); !slices.Equal(got, []string{"authorization_code", "refresh_token"}) {
		t.Fatalf("grants in force = %v", got)
	}
	if !c.MayActForUsers() {
		t.Fatal("an active client with the exchange grant may act for users")
	}
	c.Active = false
	if c.MayActForUsers() {
		t.Fatal("a client that is turned off acts for no one")
	}
	if (&Client{Active: true, GrantTypes: []string{"authorization_code"}}).MayActForUsers() {
		t.Fatal("a client without the exchange grant acts for no one")
	}
}
