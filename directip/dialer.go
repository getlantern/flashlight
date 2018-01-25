package directip

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.directip")
)

type DialerFactory struct {
	directDomains map[string]*directIps
}

func NewDialerFactory() *DialerFactory {
	return &DialerFactory{directDomains: defaultDirectDomains}
}

// NewDialer creates a new direct IP dialer that uses the given passthrough if
// it can't dial directly.
func (df *DialerFactory) NewDialer(passthrough func(ctx context.Context, network string, addr string) (net.Conn, error)) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	d := &dialer{
		directDomains: df.directDomains,
		passthrough:   passthrough,
	}
	return d.dialContext
}

type directIps struct {
	ips map[string]bool
	mx  sync.RWMutex
}

type dialer struct {
	directDomains map[string]*directIps
	passthrough   func(ctx context.Context, network string, addr string) (net.Conn, error)
}

func (d *dialer) dialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// can't split host and port, just pass through
		return d.passthrough(ctx, network, addr)
	}

	parts := strings.Split(host, ".")
	for i := len(parts) - 2; i >= 0; i-- {
		candidate := strings.ToLower(strings.Join(parts[i:], "."))
		log.Debugf("Checking %v", candidate)
		domains := d.directDomains[candidate]
		if domains != nil {
			conn, ip, err := dialDirect(ctx, network, port, domains)
			if err == nil {
				log.Debugf("Dialed %v directly as %v to %v", host, candidate, ip)
				return conn, err
			}
			log.Errorf("Unable to dial %v directly under any of its IPs: %v", host, err)
		}
	}

	// Unable to dial direct, pass through
	return d.passthrough(ctx, network, addr)
}

func dialDirect(ctx context.Context, network string, port string, domains *directIps) (conn net.Conn, ip string, err error) {
	domains.mx.RLock()
	ips := domains.ips
	domains.mx.RUnlock()

	// Clean up bad ips before exiting
	ipsToRemove := make(map[string]bool)
	defer func() {
		if len(ipsToRemove) > 0 {
			domains.mx.Lock()
			ipsCopy := make(map[string]bool, len(domains.ips)-len(ipsToRemove))
			for ip := range domains.ips {
				if !ipsToRemove[ip] {
					ipsCopy[ip] = true
				}
			}
			domains.ips = ipsCopy
			domains.mx.Unlock()
		}
	}()

	for ip = range ips {
		conn, err = net.Dial(network, fmt.Sprintf("%v:%v", ip, port))
		if err == nil {
			return
		}
		// TODO: use smarter detection here (similar to detour)
		domains.mx.Lock()
		ipsToRemove[ip] = true
	}

	return
}

var (
	defaultDirectDomains = map[string]*directIps{
		"googlevideo.com": &directIps{
			ips: map[string]bool{
				"130.211.5.254": true,
			},
		},
	}
)
