package webhook

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ErrBlockedDestination refuses a connection to an address a webhook may not
// reach.
var ErrBlockedDestination = errors.New("destination address is not allowed for webhooks")

// cgnat is 100.64.0.0/10, shared address space that cloud networks use
// internally; net.IP.IsPrivate does not cover it.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// blockedIP reports whether ip belongs to this host or a private network:
// loopback, private and shared ranges, link-local (which includes cloud
// metadata endpoints), multicast and the unspecified address.
func blockedIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsUnspecified() || cgnat.Contains(ip)
}

// deliveryClient sends webhook deliveries. Unless private targets are
// allowed, it checks every address it connects to after name resolution, so
// neither a hostname that resolves inward nor a later DNS answer reaches this
// host or its network. It ignores proxy settings and does not follow
// redirects: a receiver answers for itself, and a redirect counts as a failed
// attempt.
func deliveryClient(allowPrivateTargets bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if !allowPrivateTargets {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedDestination
			}
			ip := net.ParseIP(host)
			if ip == nil || blockedIP(ip) {
				return ErrBlockedDestination
			}
			return nil
		}
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
