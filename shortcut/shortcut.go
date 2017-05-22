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
)

type Shortcut struct {
	// sc is the shortcut actually in use. It's the same as configured if
	// enableSC is true.
	sc     shortcut.Shortcut
	mu     sync.RWMutex
	enable func() bool
}

type nullShortcut struct{}

func (s nullShortcut) Allow(string) bool {
	return false
}

func New(enable func() bool) *Shortcut {
	return &Shortcut{sc: nullShortcut{}, enable: enable}
}

func (s *Shortcut) Configure(country string) {
	country = strings.ToLower(country)
	v4, v4err := Asset("resources/" + country + "_ipv4.txt")
	v6, v6err := Asset("resources/" + country + "_ipv6.txt")
	if v4err != nil || v6err != nil {
		log.Debugf("No shortcut list for country %s", country)
		return
	}

	sc := shortcut.NewFromReader(
		bytes.NewReader(v4),
		bytes.NewReader(v6),
	)
	s.mu.Lock()
	s.sc = sc
	log.Debugf("Loaded shortcut list for country %s", country)
	s.mu.Unlock()
}

func (s *Shortcut) Allow(addr string) bool {
	if !s.enable() {
		return false
	}
	s.mu.RLock()
	_sc := s.sc
	s.mu.RUnlock()
	return _sc.Allow(addr)
}
