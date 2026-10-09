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

// blockedNets are the ranges a webhook may not reach beyond what net.IP's
// own predicates name: shared, benchmarking and reserved IPv4 space, and the
// IPv6 forms that carry an IPv4 address (NAT64, 6to4, Teredo, IPv4-compatible)
// and so could lead back to a private one.
var blockedNets = func() []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range []string{
		"0.0.0.0/8",       // "this network"
		"100.64.0.0/10",   // shared address space
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // documentation
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // documentation
		"203.0.113.0/24",  // documentation
		"240.0.0.0/4",     // reserved, and broadcast
		"::/96",           // IPv4-compatible
		"64:ff9b::/96",    // NAT64
		"64:ff9b:1::/48",  // local-use NAT64
		"2001::/32",       // Teredo
		"2001:db8::/32",   // documentation
		"2002::/16",       // 6to4
		"100::/64",        // discard-only
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}()

// blockedIP reports whether ip belongs to this host, a private network or a
// range that is not a public destination: loopback, private, link-local
// (which includes cloud metadata endpoints), multicast, unspecified, and
// blockedNets.
func blockedIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
