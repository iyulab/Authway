package client

import "testing"

// A client nobody can sign in to is not a configuration — and the empty list
// used to be stored and read back as the default (email and Google).
func TestValidateSignInMethods(t *testing.T) {
	for _, c := range []struct {
		name    string
		methods []string
		refused bool
	}{
		{"not named", nil, false},
		{"empty", []string{}, true},
		{"known", []string{"email", "magic_link", "google"}, false},
		{"unknown", []string{"email", "facebook"}, true},
	} {
		cerr := validateSignInMethods(c.methods)
		if (cerr != nil) != c.refused {
			t.Errorf("%s: refused = %v, want %v", c.name, cerr != nil, c.refused)
		}
		if cerr != nil && (cerr.Code != "invalid_enabled_auth_providers" || cerr.Field != "enabled_auth_providers") {
			t.Errorf("%s: %+v, want invalid_enabled_auth_providers on enabled_auth_providers", c.name, cerr)
		}
	}
}
