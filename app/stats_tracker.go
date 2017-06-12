package app

import (
	"sync"

	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ws"
)

type statsTracker struct {
	stats.StatsTracker
	service ws.Service
}

func New() *statsTracker {
	st := &statsTracker{}
	st.Broadcast = func(s stats.Stats) {
		select {
		case st.service.out <- s:
			// ok
		default:
			// don't block if no-one is listening
		}
	}
}

func (s *statsTracker) StartService() error {
	helloFn := func(write func(interface{})) {
		log.Debugf("Sending Lantern stats to new client")
		s.mu.Lock()
		st := s.stats
		s.mu.Unlock()
		write(st)
	}

	_, err := ws.Register("stats", helloFn)
	return err
}

func (s *statsTracker) StopService() {
	ws.Unregister("stats")
}
