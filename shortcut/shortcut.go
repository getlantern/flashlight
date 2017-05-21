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
	sc shortcut.Shortcut
	// configured is the shortcut loaded based on the country.
	configured shortcut.Shortcut
	enableSC   bool
	mu         sync.RWMutex
}

type nullShortcut struct{}

func (s nullShortcut) Allow(string) bool {
	return false
}

func New() *Shortcut {
	return &Shortcut{sc: nullShortcut{}, configured: nullShortcut{}}
}

func (s *Shortcut) Enable(enable bool) {
	s.mu.Lock()
	s.enableSC = enable
	if enable {
		s.sc = s.configured
	} else {
		s.sc = nullShortcut{}
	}
	s.mu.Unlock()
	log.Debugf("Done enable/disable shortcut. Enabled %v", enable)
}

func (s *Shortcut) Configure(country string) {
	country = strings.ToLower(country)
	v4, v4err := Asset("resources/" + country + "_ipv4.txt")
	v6, v6err := Asset("resources/" + country + "_ipv6.txt")
	if v4err != nil || v6err != nil {
		log.Debugf("No shortcut list for country %s", country)
		return
	}

	s.configured = shortcut.NewFromReader(
		bytes.NewReader(v4),
		bytes.NewReader(v6),
	)
	s.mu.Lock()
	if s.enableSC {
		s.sc = s.configured
		log.Debugf("Loaded shortcut list for country %s", country)
	}
	s.mu.Unlock()
}

func (s *Shortcut) Allow(addr string) bool {
	s.mu.RLock()
	_sc := s.sc
	s.mu.RUnlock()
	return _sc.Allow(addr)
}
