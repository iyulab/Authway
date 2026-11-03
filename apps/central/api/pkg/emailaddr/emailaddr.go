// Package emailaddr is the one place an email address is put into the form in
// which it is stored and compared.
package emailaddr

import "strings"

// Normalize returns the form an address is stored and compared in: without
// surrounding whitespace and with ASCII capitals in lower case. People type the
// same address in different cases, and identity providers return it in lower
// case, so an address that differs only in the case of its ASCII letters is the
// same address.
//
// Only ASCII letters are folded. Unicode case folding makes different
// addresses equal — the Kelvin sign U+212A folds to "k" — and a domain written
// with one would then pass for another organisation's. Characters outside
// ASCII are kept as they are, so such an address stays itself.
func Normalize(address string) string {
	b := []byte(strings.TrimSpace(address))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// Equal reports whether two addresses are the same address.
func Equal(a, b string) bool {
	return Normalize(a) == Normalize(b)
}
