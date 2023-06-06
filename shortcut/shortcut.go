// package shortcut loads country specific shortcut subnet list from resources
// so the caller can check if an IP should be dialed directly or not.  If
// there's no list for the country, a default list which includes private IP
// ranges are used.
package shortcut

import (
	"bytes"
	"context"
	"embed"
	"net"
	"strings"
	"sync"

	"github.com/getlantern/golog"
	"github.com/getlantern/netx"
	"github.com/getlantern/shortcut"

	geolookup "github.com/getlantern/flashlight/v7/geolookup"
)

var (
	log = golog.LoggerFor("flashlight.shortcut")

	sc shortcut.Shortcut = &nullShortcut{}
	mu sync.RWMutex
)

func init() {
	go func() {
		for <-geolookup.OnRefresh() {
			configure(geolookup.GetCountry(0))
		}
	}()
}

type nullShortcut struct{}

func (s *nullShortcut) RouteMethod(context.Context, string) (shortcut.Method, net.IP) {
	return shortcut.Proxy, nil
}

func (s *nullShortcut) SetResolver(func(context.Context, string) (net.IP, error)) {
}

//go:embed resources/*.txt
var ipRanges embed.FS

func configure(country string) {
	country = strings.ToLower(country)
	var _sc shortcut.Shortcut
	for {
		v4, v4err := ipRanges.ReadFile("resources/" + country + "_ipv4.txt")
		v6, v6err := ipRanges.ReadFile("resources/" + country + "_ipv6.txt")
		v4Proxied, v4errProxied := ipRanges.ReadFile("resources/" + country + "_ipv4_proxied.txt")
		v6Proxied, v6errProxied := ipRanges.ReadFile("resources/" + country + "_ipv6_proxied.txt")
		if v4err == nil && v6err == nil && v4errProxied == nil && v6errProxied == nil {
			log.Debugf("Configuring for country %v", country)
			_sc = shortcut.NewFromReader(
				bytes.NewReader(v4),
				bytes.NewReader(v6),
				bytes.NewReader(v4Proxied),
				bytes.NewReader(v6Proxied),
			)
			_sc.SetResolver(func(ctx context.Context, addr string) (net.IP, error) {
				// TODO: change netx to accept context
				done := make(chan struct{})
				var tcpAddr *net.TCPAddr
				var err error
				go func() {
					tcpAddr, err = netx.Resolve("tcp", addr)
					select {
					case done <- struct{}{}:
					default:
					}
				}()
				select {
				case <-done:
					if err != nil {
						return nil, err
					}
					return tcpAddr.IP, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})
			break
		} else {
			log.Debugf("Could not open all files %v, %v, %v, %v", v4err, v6err, v4errProxied, v6errProxied)
		}
		log.Debugf("no shortcut list for country %s, fallback to default", country)
		country = "default"
	}

	mu.Lock()
	sc = _sc
	mu.Unlock()
	log.Debugf("loaded shortcut list for country %s", country)
}

func Allow(ctx context.Context, addr string) (shortcut.Method, net.IP) {
	mu.RLock()
	_sc := sc
	mu.RUnlock()
	return _sc.RouteMethod(ctx, addr)
}
