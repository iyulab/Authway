package tenant

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// password_min_length can raise the least length, not lower it; zero asks
// for nothing more.
func TestTenantSettings_PasswordMinLengthIsBounded(t *testing.T) {
	validate := validator.New()
	for length, ok := range map[int]bool{-1: false, 0: true, 1: false, 7: false, 8: true, 12: true, 72: true, 73: false} {
		settings := TenantSettings{PasswordMinLength: length}
		if err := validate.Struct(UpdateTenantRequest{Settings: &settings}); (err == nil) != ok {
			t.Errorf("password_min_length %d: %v — want accepted=%v", length, err, ok)
		}
	}
}

// allowed_domains names domains, and decides which addresses an account can
// be created for.
func TestTenantSettings_AllowedDomains(t *testing.T) {
	validate := validator.New()
	for _, c := range []struct {
		domains []string
		ok      bool
	}{
		{nil, true}, {[]string{}, true}, {[]string{"example.com"}, true}, {[]string{"example.com", "mail.example.org"}, true},
		{[]string{"@example.com"}, false}, {[]string{"someone@example.com"}, false}, {[]string{"not a domain"}, false}, {[]string{""}, false},
	} {
		settings := TenantSettings{AllowedDomains: c.domains}
		if err := validate.Struct(UpdateTenantRequest{Settings: &settings}); (err == nil) != c.ok {
			t.Errorf("allowed_domains %q: %v — want accepted=%v", c.domains, err, c.ok)
		}
	}

	for _, c := range []struct {
		domains []string
		email   string
		want    bool
	}{
		{nil, "a@anywhere.example", true},
		{[]string{}, "a@anywhere.example", true},
		{[]string{"", " "}, "a@anywhere.example", true},
		{[]string{"example.com"}, "a@example.com", true},
		{[]string{"example.com"}, "a@Example.COM", true},
		{[]string{"example.org", "example.com"}, "a@example.com", true},
		{[]string{"example.com"}, "a@mail.example.com", false},
		{[]string{"example.com"}, "a@notexample.com", false},
		{[]string{"example.com"}, "example.com", false},
		{[]string{"example.com"}, "a@example.com@elsewhere.example", false},
	} {
		if got := (TenantSettings{AllowedDomains: c.domains}).AllowsAddress(c.email); got != c.want {
			t.Errorf("AllowsAddress(%q) under %q = %v, want %v", c.email, c.domains, got, c.want)
		}
	}
}
