package shortcut

import (
	"bytes"
	"strings"
	"sync"

	"github.com/getlantern/golog"
	"github.com/getlantern/shortcut"
)

var (
	log = golog.LoggerFor("flashlight.shortcut")

	sc shortcut.Shortcut = &nullShortcut{}
	mu sync.RWMutex
)

type nullShortcut struct{}

func (s *nullShortcut) Allow(string) bool {
	return false
}

func Configure(country string) {
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
