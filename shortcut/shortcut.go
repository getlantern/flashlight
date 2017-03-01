package shortcut

import (
	"bytes"
	"strings"
	"sync/atomic"

	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/golog"
	"github.com/getlantern/shortcut"
)

var (
	log = golog.LoggerFor("flashlight.shortcut")

	sc atomic.Value
)

func init() {
	sc.Store(&nullShortcut{})
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

	sc.Store(shortcut.NewFromReader(
		bytes.NewReader(v4),
		bytes.NewReader(v6),
	))
}

func Allow(addr string) bool { return sc.Load().(shortcut.Shortcut).Allow(addr) }
