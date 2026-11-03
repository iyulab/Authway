package emailaddr

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"alice@corp.com":         "alice@corp.com",
		"Alice@Corp.com":         "alice@corp.com",
		"  ALICE@CORP.COM \t":    "alice@corp.com",
		"first.last+tag@Corp.IO": "first.last+tag@corp.io",
		"":                       "",
		// Not folded: they are other letters, and an address that spells one
		// is another address.
		"K@corp.com":       "K@corp.com", // Kelvin sign, not "k"
		"alice@İzmir.tr":   "alice@İzmir.tr",
		"alice@CORP.ſo.io": "alice@corp.ſo.io", // long s, not "s"
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEqual_IsOnlyAboutTheCaseOfASCIILetters(t *testing.T) {
	if !Equal("Alice@Corp.com", " alice@CORP.com ") {
		t.Error("addresses that differ in ASCII case must be equal")
	}
	if Equal("k@corp.com", "K@corp.com") {
		t.Error("the Kelvin sign must not pass for \"k\"")
	}
	if Equal("alice@bok.com", "alice@boK.com") {
		t.Error("a domain spelled with the Kelvin sign must not pass for the ASCII one")
	}
}
