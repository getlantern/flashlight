// package shortcut determines whether traffic should be routed directly, should
// be proxied, or whether the routing is unknown based on matching (or not)
// known IP ranges for DNS poisoned IP ranges in specific countries, for
// example.

package shortcut

import (
	"bufio"
	"context"
	"io"
	"net"
)

type Method int

const (
	// Proxy indicates traffic to this domain/IP should be proxied.
	Proxy Method = iota

	// Direct indicates traffic to this domain/IP should be routed directly.
	Direct

	// Unknown indicates shortcut has no opinion about the routing for this domain/IP.
	Unknown
)

type Shortcut interface {
	// RouteMethod checks if the address should be routed directly or should
	// be proxied, or whether the ideal method is unknown.
	RouteMethod(ctx context.Context, addr string) (Method, net.IP)
	// SetResolver sets a custom resolver to replace the system default.
	SetResolver(r func(ctx context.Context, addr string) (net.IP, error))
}

type shortcut struct {
	v4DirectList *SortList
	v6DirectList *SortList
	v4ProxyList  *SortList
	v6ProxyList  *SortList
	resolver     func(ctx context.Context, addr string) (net.IP, error)
}

// NewFromReader is a helper to create shortcut from readers. The content
// should be in CIDR format, one entry per line.
func NewFromReader(v4Direct, v6Direct, v4Proxy, v6Proxy io.Reader) Shortcut {
	return New(readLines(v4Direct), readLines(v6Direct), readLines(v4Proxy), readLines(v6Proxy))
}

func readLines(r io.Reader) []string {
	lines := []string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

// New creates a new shortcut from the subnets for both direct and proxied traffic.
func New(ipv4DirectSubnets, ipv6DirectSubnets, ipv4ProxySubnets, ipv6ProxySubnets []string) Shortcut {
	log.Debugf("Creating shortcut with direct %d ipv4 subnets and %d ipv6 subnets and proxied %d ipv4 subnets and %d ipv6 subnets",
		len(ipv4DirectSubnets),
		len(ipv6DirectSubnets),
		len(ipv4ProxySubnets),
		len(ipv6ProxySubnets),
	)
	return &shortcut{
		v4DirectList: NewSortList(ipv4DirectSubnets),
		v6DirectList: NewSortList(ipv6DirectSubnets),
		v4ProxyList:  NewSortList(ipv4ProxySubnets),
		v6ProxyList:  NewSortList(ipv6ProxySubnets),
		resolver:     defaultResolver,
	}
}

func defaultResolver(ctx context.Context, addr string) (net.IP, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if ip := addr.IP.To4(); ip != nil {
			return ip, nil
		}
		if ip := addr.IP.To16(); ip != nil {
			return ip, nil
		}
	}
	return nil, err
}

func (s *shortcut) SetResolver(r func(ctx context.Context, addr string) (net.IP, error)) {
	s.resolver = r
}

func (s *shortcut) RouteMethod(ctx context.Context, addr string) (Method, net.IP) {
	ip, err := s.resolver(ctx, addr)
	if err != nil {
		return Unknown, nil
	}
	if ip4 := ip.To4(); ip4 != nil {
		return s.check(s.v4DirectList, s.v4ProxyList, ip4)
	}
	if ip6 := ip.To16(); ip6 != nil {
		return s.check(s.v6DirectList, s.v6ProxyList, ip6)
	}
	return Unknown, ip
}

func (s *shortcut) check(direct, proxy *SortList, ip net.IP) (Method, net.IP) {
	if direct.Contains(ip) {
		return Direct, ip
	}
	if proxy.Contains(ip) {
		return Proxy, ip
	}
	return Unknown, ip
}
