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
				"74.125.0.59":   true,
				"74.125.1.42":   true,
				"74.125.1.46":   true,
				"74.125.1.47":   true,
				"74.125.1.49":   true,
				"74.125.1.54":   true,
				"74.125.1.52":   true,
				"74.125.1.140":  true,
				"74.125.1.89":   true,
				"74.125.1.83":   true,
				"74.125.1.81":   true,
				"74.125.1.105":  true,
				"74.125.1.137":  true,
				"74.125.1.111":  true,
				"74.125.1.152":  true,
				"74.125.1.151":  true,
				"74.125.1.211":  true,
				"74.125.3.28":   true,
				"74.125.3.27":   true,
				"74.125.3.71":   true,
				"74.125.3.104":  true,
				"74.125.3.108":  true,
				"74.125.3.176":  true,
				"74.125.3.179":  true,
				"74.125.3.183":  true,
				"74.125.3.91":   true,
				"74.125.3.169":  true,
				"74.125.3.202":  true,
				"74.125.3.204":  true,
				"74.125.3.208":  true,
				"74.125.3.242":  true,
				"74.125.3.239":  true,
				"74.125.3.214":  true,
				"74.125.3.210":  true,
				"74.125.3.247":  true,
				"74.125.4.23":   true,
				"74.125.4.42":   true,
				"74.125.4.40":   true,
				"74.125.4.58":   true,
				"74.125.4.112":  true,
				"74.125.4.103":  true,
				"74.125.5.14":   true,
				"74.125.5.17":   true,
				"74.125.5.24":   true,
				"74.125.5.19":   true,
				"74.125.5.23":   true,
				"74.125.5.49":   true,
				"74.125.5.42":   true,
				"74.125.5.50":   true,
				"74.125.5.53":   true,
				"74.125.5.134":  true,
				"74.125.5.138":  true,
				"74.125.5.143":  true,
				"74.125.5.168":  true,
				"74.125.5.166":  true,
				"74.125.5.175":  true,
				"74.125.5.177":  true,
				"74.125.5.200":  true,
				"74.125.5.208":  true,
				"74.125.5.209":  true,
				"74.125.5.214":  true,
				"74.125.5.144":  true,
				"74.125.5.230":  true,
				"74.125.5.240":  true,
				"74.125.6.81":   true,
				"74.125.6.106":  true,
				"74.125.6.118":  true,
				"74.125.6.140":  true,
				"74.125.6.136":  true,
				"74.125.6.171":  true,
				"74.125.6.184":  true,
				"74.125.6.201":  true,
				"74.125.6.214":  true,
				"74.125.6.217":  true,
				"74.125.7.9":    true,
				"74.125.7.17":   true,
				"74.125.7.20":   true,
				"74.125.7.50":   true,
				"74.125.7.51":   true,
				"74.125.7.124":  true,
				"74.125.7.140":  true,
				"74.125.7.151":  true,
				"74.125.7.150":  true,
				"74.125.7.176":  true,
				"74.125.7.212":  true,
				"74.125.7.214":  true,
				"74.125.7.152":  true,
				"74.125.7.235":  true,
				"74.125.7.234":  true,
				"74.125.7.237":  true,
				"74.125.8.57":   true,
				"74.125.8.58":   true,
				"74.125.8.79":   true,
				"74.125.8.81":   true,
				"74.125.8.108":  true,
				"74.125.8.83":   true,
				"74.125.8.76":   true,
				"74.125.8.118":  true,
				"74.125.8.119":  true,
				"74.125.8.139":  true,
				"74.125.8.152":  true,
				"74.125.8.156":  true,
				"74.125.8.185":  true,
				"74.125.8.195":  true,
				"74.125.8.197":  true,
				"74.125.8.204":  true,
				"74.125.8.203":  true,
				"74.125.8.208":  true,
				"74.125.8.202":  true,
				"74.125.8.210":  true,
				"74.125.8.206":  true,
				"74.125.8.211":  true,
				"74.125.8.221":  true,
				"74.125.8.217":  true,
				"74.125.8.214":  true,
				"74.125.8.222":  true,
				"74.125.9.15":   true,
				"74.125.9.21":   true,
				"74.125.9.23":   true,
				"74.125.9.38":   true,
				"74.125.9.44":   true,
				"74.125.9.54":   true,
				"74.125.9.75":   true,
				"74.125.9.78":   true,
				"74.125.9.104":  true,
				"74.125.9.113":  true,
				"74.125.9.138":  true,
				"74.125.9.140":  true,
				"74.125.9.142":  true,
				"74.125.9.141":  true,
				"74.125.9.144":  true,
				"74.125.9.145":  true,
				"74.125.9.173":  true,
				"74.125.9.174":  true,
				"74.125.9.177":  true,
				"74.125.9.180":  true,
				"74.125.9.207":  true,
				"74.125.9.212":  true,
				"74.125.9.236":  true,
				"74.125.9.241":  true,
				"74.125.9.247":  true,
				"74.125.10.8":   true,
				"74.125.10.10":  true,
				"74.125.10.40":  true,
				"74.125.10.44":  true,
				"74.125.10.58":  true,
				"74.125.10.71":  true,
				"74.125.10.75":  true,
				"74.125.10.91":  true,
				"74.125.10.104": true,
				"74.125.10.136": true,
				"74.125.10.138": true,
				"74.125.10.137": true,
				"74.125.10.139": true,
				"74.125.10.140": true,
				"74.125.10.151": true,
				"74.125.10.154": true,
				"74.125.10.153": true,
				"74.125.10.156": true,
				"74.125.10.168": true,
				"74.125.10.169": true,
				"74.125.10.170": true,
				"74.125.10.172": true,
				"74.125.10.186": true,
				"74.125.10.187": true,
				"74.125.10.188": true,
				"74.125.10.200": true,
				"74.125.10.199": true,
				"74.125.10.203": true,
				"74.125.10.152": true,
				"74.125.10.232": true,
				"74.125.10.234": true,
				"74.125.10.235": true,
				"74.125.10.236": true,
				"74.125.11.10":  true,
				"74.125.11.44":  true,
				"74.125.11.60":  true,
				"74.125.11.57":  true,
				"74.125.11.55":  true,
				"74.125.11.134": true,
				"74.125.11.141": true,
				"74.125.11.144": true,
				"74.125.11.151": true,
				"74.125.11.152": true,
				"74.125.11.172": true,
				"74.125.11.176": true,
				"74.125.11.207": true,
				"74.125.11.212": true,
				"74.125.11.215": true,
				"74.125.11.208": true,
				"74.125.11.230": true,
				"74.125.11.232": true,
				"74.125.11.235": true,
				"74.125.11.236": true,
				"74.125.11.241": true,
				"74.125.11.243": true,
				"74.125.11.244": true,
				"74.125.11.247": true,
				"74.125.12.26":  true,
				"74.125.12.59":  true,
				"74.125.12.176": true,
				"74.125.12.201": true,
				"74.125.12.208": true,
				"74.125.12.216": true,
				"74.125.12.233": true,
				"74.125.12.249": true,
				"74.125.12.234": true,
				"74.125.13.7":   true,
				"74.125.13.10":  true,
				"74.125.13.12":  true,
				"74.125.13.13":  true,
				"74.125.13.47":  true,
				"74.125.13.48":  true,
				"74.125.13.53":  true,
				"74.125.13.75":  true,
				"74.125.13.77":  true,
				"74.125.13.106": true,
				"74.125.13.110": true,
				"74.125.13.105": true,
				"74.125.13.129": true,
				"74.125.13.136": true,
				"74.125.13.148": true,
				"74.125.13.164": true,
				"74.125.13.171": true,
				"74.125.13.183": true,
				"74.125.13.194": true,
				"74.125.13.213": true,
				"74.125.13.219": true,
				"74.125.13.234": true,
				"74.125.15.4":   true,
				"74.125.15.14":  true,
				"74.125.15.24":  true,
				"74.125.15.31":  true,
				"74.125.15.15":  true,
				"74.125.15.35":  true,
				"74.125.15.29":  true,
				"74.125.15.18":  true,
				"74.125.15.70":  true,
				"74.125.15.68":  true,
				"74.125.15.73":  true,
				"74.125.15.77":  true,
				"74.125.15.86":  true,
				"74.125.15.88":  true,
				"74.125.15.94":  true,
				"74.125.15.96":  true,
				"74.125.15.89":  true,
				"74.125.15.130": true,
				"74.125.15.132": true,
				"74.125.15.145": true,
				"74.125.15.153": true,
				"74.125.15.150": true,
				"74.125.15.157": true,
				"74.125.15.155": true,
				"74.125.15.156": true,
				"74.125.15.161": true,
				"74.125.15.193": true,
				"74.125.15.203": true,
				"74.125.15.206": true,
				"74.125.15.209": true,
				"74.125.15.211": true,
				"74.125.15.210": true,
				"74.125.15.217": true,
				"74.125.15.220": true,
				"74.125.15.226": true,
				"74.125.15.227": true,
			},
		},
	}
)
