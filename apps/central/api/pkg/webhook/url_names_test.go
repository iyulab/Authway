package webhook

import (
	"context"
	"net"
	"testing"

	"authway/apps/central/api/pkg/outbound"
)

type fixedNames map[string]string

func (f fixedNames) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ip, ok := f[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
}

// A receiver whose host name resolves to a local or private address is
// refused when it is registered — as one named by such an address is —
// rather than accepted and then failing every delivery.
func TestValidateURL_ResolvesTheHost(t *testing.T) {
	s := &service{names: fixedNames{"public.example.com": "93.184.216.34", "inward.example.com": "10.0.0.5"}}
	for raw, refused := range map[string]bool{
		"https://public.example.com/hook":  false,
		"https://inward.example.com/hook":  true,
		"https://10.0.0.5/hook":            true,
		"https://unknown.example.com/hook": false, // nothing to say yet; the delivery checks when it connects
	} {
		if err := s.validateURL(raw); (err != nil) != refused {
			t.Errorf("%s: refused = %v, want %v", raw, err != nil, refused)
		}
	}
	lax := &service{policy: outbound.Policy{AllowPrivate: true}, names: fixedNames{"inward.example.com": "10.0.0.5"}}
	if err := lax.validateURL("https://inward.example.com/hook"); err != nil {
		t.Errorf("a deployment that allows private targets refused one: %v", err)
	}
}
