// Package outbound decides which addresses this deployment may call when the
// address was given to it by someone else — a webhook's URL, the address a
// client is told about a logout at. Such a call starts inside the
// deployment's own network, so an address in there is not just another
// destination: it is how a caller reaches what it could not reach itself.
package outbound

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

// ErrRefused refuses an address the deployment may not call.
var ErrRefused = errors.New("destination address is not allowed")

// Policy says which addresses may be called.
type Policy struct {
	// AllowPrivate lets calls reach this host and private networks, for
	// development and tests where receivers run next to the deployment.
	// Addresses that are no one's server stay refused whatever it says:
	// link-local (which includes cloud metadata endpoints), multicast,
	// unspecified and reserved ranges.
	AllowPrivate bool
}

// never are ranges that are not a destination for anyone, beyond what
// net.IP's own predicates name: "this network", reserved and documentation
// space, and the IPv6 forms that carry an IPv4 address (NAT64, 6to4, Teredo,
// IPv4-compatible) and so could lead back to an address refused above.
var never = cidrs(
	"0.0.0.0/8",       // "this network"
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
)

// private are ranges inside a network of the deployment's own, beyond
// loopback and what net.IP.IsPrivate names: shared address space, which
// carrier-grade NAT and overlay networks use.
var private = cidrs("100.64.0.0/10")

func cidrs(list ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(list))
	for _, cidr := range list {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}

func in(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// RefusesIP reports whether ip may not be called.
func (p Policy) RefusesIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	// Loopback first: ::1 also lies in the IPv4-compatible range below.
	if ip.IsLoopback() || ip.IsPrivate() || in(private, ip) {
		return !p.AllowPrivate
	}
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || in(never, ip)
}

// RefusesHost reports whether a URL's host, as written, names an address that
// may not be called: an IP address RefusesIP refuses, or this host by name.
// A name that only resolves to such an address is not caught here.
func (p Policy) RefusesHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip := net.ParseIP(host); ip != nil {
		return p.RefusesIP(ip)
	}
	return !p.AllowPrivate && (host == "localhost" || strings.HasSuffix(host, ".localhost"))
}

// Resolver looks a host name up. *net.Resolver is one.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// resolveTimeout bounds the lookup RefusesName makes while a request waits.
const resolveTimeout = 2 * time.Second

// RefusesName reports whether host, looked up now, has an address that may
// not be called. A name that does not resolve is not refused: nothing can be
// said about it, and whoever makes the call has to check again when it
// connects — a name can also change what it resolves to afterwards.
func (p Policy) RefusesName(ctx context.Context, resolver Resolver, host string) bool {
	if p.RefusesHost(host) {
		return true
	}
	if net.ParseIP(host) != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if p.RefusesIP(addr.IP) {
			return true
		}
	}
	return false
}
