package handler

import (
	"slices"
	"testing"
)

// What a consent grants is what the user approved of what the client asked
// for: the screen cannot add a scope the request did not name.
func TestGrantedOfIsTheApprovedPartOfTheRequest(t *testing.T) {
	requested := []string{"openid", "profile", "delegation"}
	cases := map[string]struct{ approved, want []string }{
		"everything asked":        {[]string{"openid", "profile", "delegation"}, []string{"openid", "profile", "delegation"}},
		"some of it":              {[]string{"openid"}, []string{"openid"}},
		"a scope never asked for": {[]string{"openid", "admin"}, []string{"openid"}},
		"the same scope twice":    {[]string{"openid", "openid"}, []string{"openid"}},
		"nothing":                 {nil, []string{}},
	}
	for name, tc := range cases {
		if got := grantedOf(tc.approved, requested); !slices.Equal(got, tc.want) {
			t.Errorf("%s: granted %v, want %v", name, got, tc.want)
		}
	}
}

func TestIDTokenWithAddsTheHandleOnlyWhenThereIsOne(t *testing.T) {
	if _, ok := idTokenWith(map[string]any{}, "")["delegation_grant"]; ok {
		t.Fatal("no delegation, no claim")
	}
	if got := idTokenWith(map[string]any{}, "h")["delegation_grant"]; got != "h" {
		t.Fatalf("delegation_grant = %v", got)
	}
}
