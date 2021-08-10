// package shortcut loads country specific shortcut subnet list from resources
// so the caller can check if an IP should be dialed directly or not.  If
// there's no list for the country, a default list which includes private IP
// ranges are used.
package shortcut

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"

	"github.com/getlantern/golog"
	"github.com/getlantern/netx"
	"github.com/getlantern/shortcut"

	"github.com/getlantern/flashlight/geolookup"
)

var (
	log = golog.LoggerFor("flashlight.shortcut")

	sc shortcut.Shortcut = &nullShortcut{}
	mu sync.RWMutex
)

func init() {
	go func() {
		for <-geolookup.DefaultInstance.OnRefresh() {
			configure(geolookup.DefaultInstance.GetCountry(0))
		}
	}()
}

type nullShortcut struct{}

func (s *nullShortcut) Allow(context.Context, string) (bool, net.IP) {
	return false, nil
}

func (s *nullShortcut) SetResolver(func(context.Context, string) (net.IP, error)) {
}

func configure(country string) {
	country = strings.ToLower(country)
	var _sc shortcut.Shortcut
	for {
		v4, v4err := Asset("resources/" + country + "_ipv4.txt")
		v6, v6err := Asset("resources/" + country + "_ipv6.txt")
		if v4err == nil && v6err == nil {
			_sc = shortcut.NewFromReader(
				bytes.NewReader(v4),
				bytes.NewReader(v6),
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
		}
		log.Debugf("no shortcut list for country %s, fallback to default", country)
		country = "default"
	}

	mu.Lock()
	sc = _sc
	mu.Unlock()
	log.Debugf("loaded shortcut list for country %s", country)
}

func Allow(ctx context.Context, addr string) (bool, net.IP) {
	mu.RLock()
	_sc := sc
	mu.RUnlock()
	return _sc.Allow(ctx, addr)
}
