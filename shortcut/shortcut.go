// package shortcut loads country specific shortcut subnet list from resources
// so the caller can check if an IP should be dialed directly or not.  If
// there's no list for the country, a default list which includes private IP
// ranges are used.
package shortcut

import (
	"bytes"
	"net"
	"strings"
	"sync"

	"github.com/getlantern/shortcut"

	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
)

var (
	log = logging.LoggerFor("flashlight.shortcut")

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

func (s *nullShortcut) Allow(string) (bool, net.IP) {
	return false, nil
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
			break
		}
		log.Infof("no shortcut list for country %s, fallback to default", country)
		country = "default"
	}

	mu.Lock()
	sc = _sc
	mu.Unlock()
	log.Infof("loaded shortcut list for country %s", country)
}

func Allow(addr string) (bool, net.IP) {
	mu.RLock()
	_sc := sc
	mu.RUnlock()
	return _sc.Allow(addr)
}
