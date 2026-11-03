package tenant

import "testing"

// A tenant that names its domains admits an address in one of them however the
// ASCII letters of the domain are cased — and nothing that merely folds to it:
// a domain spelled with the Kelvin sign is not the one spelled with "k".
func TestAllowsAddress_ComparesTheDomainByASCIICaseOnly(t *testing.T) {
	settings := TenantSettings{AllowedDomains: []string{"Book.example"}}
	for address, want := range map[string]bool{
		"a@book.example":      true,
		"a@BOOK.EXAMPLE":      true,
		"a@booK.example":      false,
		"a@book.example.evil": false,
		"a@sub.book.example":  false,
		"no-at-sign":          false,
	} {
		if got := settings.AllowsAddress(address); got != want {
			t.Errorf("AllowsAddress(%q) = %v, want %v", address, got, want)
		}
	}
}
