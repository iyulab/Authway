package webhook

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"

	"authway/apps/central/api/pkg/outbound"
)

// ErrBlockedDestination refuses a connection to an address a webhook may not
// reach.
var ErrBlockedDestination = errors.New("destination address is not allowed for webhooks")

// deliveryClient sends webhook deliveries. It checks every address it
// connects to after name resolution against policy, so neither a hostname
// that resolves to a refused address nor a later DNS answer gets through. It
// ignores proxy settings and does not follow redirects: a receiver answers
// for itself, and a redirect counts as a failed attempt.
func deliveryClient(policy outbound.Policy) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return ErrBlockedDestination
		}
		ip := net.ParseIP(host)
		if ip == nil || policy.RefusesIP(ip) {
			return ErrBlockedDestination
		}
		return nil
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
