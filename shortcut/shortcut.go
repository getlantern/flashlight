package shortcut

import (
	"bytes"
	"strings"
	"sync"

	"github.com/getlantern/golog"
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
		for <-geolookup.OnRefresh() {
			configure(geolookup.GetCountry(0))
		}
	}()
}

type nullShortcut struct{}

func (s *nullShortcut) Allow(string) bool {
	return false
}

func configure(country string) {
	country = strings.ToLower(country)
	v4, v4err := Asset("resources/" + country + "_ipv4.txt")
	v6, v6err := Asset("resources/" + country + "_ipv6.txt")
	if v4err != nil || v6err != nil {
		log.Debugf("no shortcut list for country %s", country)
		return
	}

	_sc := shortcut.NewFromReader(
		bytes.NewReader(v4),
		bytes.NewReader(v6),
		// Use the default net.ResolveTCPAddr.
		//
		// On Android, the DNS request will go through Tun device, and send via
		// udpgw to whichever DNS server configured on the proxy. However, the
		// host is already resolved before sending to Lantern client, so
		// it's not used anyway.
		//
		// On desktop, it resolves using local DNS, which is exactly what
		// shortcut requires.
		nil,
	)
	mu.Lock()
	sc = _sc
	mu.Unlock()
	log.Debugf("loaded shortcut list for country %s", country)
}

func Allow(addr string) bool {
	mu.RLock()
	_sc := sc
	mu.RUnlock()
	return _sc.Allow(addr)
}
