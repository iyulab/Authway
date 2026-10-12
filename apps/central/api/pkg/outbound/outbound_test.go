package outbound

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestRefusesIP(t *testing.T) {
	// What each address is: not a destination for anyone, inside a network of
	// the deployment's own, or public.
	const never, private, public = "never", "private", "public"
	for addr, kind := range map[string]string{
		"169.254.169.254": never, // cloud metadata
		"fe80::1":         never,
		"0.0.0.0":         never,
		"0.1.2.3":         never,
		"224.0.0.1":       never,
		"255.255.255.255": never,
		"198.18.0.1":      never,
		"64:ff9b::a00:1":  never,
		"2002:a00:1::1":   never,
		"2001:0:a00:1::":  never,
		"::a00:1":         never,
		"127.0.0.1":       private,
		"::1":             private,
		"10.1.2.3":        private,
		"172.16.0.1":      private,
		"192.168.1.1":     private,
		"100.100.0.229":   private,
		"fd00::1":         private,
		"::ffff:10.0.0.1": private,
		"93.184.216.34":   public,
		"2606:4700::1111": public,
	} {
		ip := net.ParseIP(addr)
		if got, want := (Policy{}).RefusesIP(ip), kind != public; got != want {
			t.Errorf("%s (%s): refused=%v by default, want %v", addr, kind, got, want)
		}
		if got, want := (Policy{AllowPrivate: true}).RefusesIP(ip), kind == never; got != want {
			t.Errorf("%s (%s): refused=%v with private targets allowed, want %v", addr, kind, got, want)
		}
	}
}

func TestRefusesHost(t *testing.T) {
	strict, lax := Policy{}, Policy{AllowPrivate: true}
	for _, host := range []string{"localhost", "LOCALHOST.", "app.localhost", "127.0.0.1", "10.0.0.5", "::1"} {
		if !strict.RefusesHost(host) {
			t.Errorf("%s: expected a refusal", host)
		}
		if lax.RefusesHost(host) {
			t.Errorf("%s: refused with private targets allowed", host)
		}
	}
	for _, host := range []string{"169.254.169.254", "0.0.0.0", "fe80::1"} {
		if !strict.RefusesHost(host) || !lax.RefusesHost(host) {
			t.Errorf("%s: must be refused whatever the policy", host)
		}
	}
	if strict.RefusesHost("hooks.example.com") {
		t.Error("a public name was refused as written")
	}
}

type fakeResolver map[string][]string

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	found, ok := f[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	addrs := make([]net.IPAddr, len(found))
	for i, a := range found {
		addrs[i] = net.IPAddr{IP: net.ParseIP(a)}
	}
	return addrs, nil
}

// A name is refused when any address it resolves to is: one inward address
// among public ones is enough to be sent there.
func TestRefusesName(t *testing.T) {
	names := fakeResolver{
		"public.example.com":  {"93.184.216.34"},
		"inward.example.com":  {"10.0.0.5"},
		"mixed.example.com":   {"93.184.216.34", "192.168.1.10"},
		"metadata.example.io": {"169.254.169.254"},
	}
	ctx := context.Background()
	for host, want := range map[string]bool{
		"public.example.com":    false,
		"inward.example.com":    true,
		"mixed.example.com":     true,
		"metadata.example.io":   true,
		"unresolved.example.io": false, // nothing can be said about it
		"10.0.0.5":              true,
		"93.184.216.34":         false,
	} {
		if got := (Policy{}).RefusesName(ctx, names, host); got != want {
			t.Errorf("%s: refused=%v, want %v", host, got, want)
		}
	}
	lax := Policy{AllowPrivate: true}
	if lax.RefusesName(ctx, names, "inward.example.com") {
		t.Error("a private address was refused with private targets allowed")
	}
	if !lax.RefusesName(ctx, names, "metadata.example.io") {
		t.Error("a metadata address must be refused whatever the policy")
	}
}
