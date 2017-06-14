package app

import (
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ws"
)

type statsTracker struct {
	stats.Tracker
	service *ws.Service
}

func NewStatsTracker() *statsTracker {
	s := &statsTracker{}
	s.Broadcast = func(st stats.Stats) {
		select {
		case s.service.Out <- st:
			// ok
		default:
			// don't block if no-one is listening
		}
	}
	return s
}

func (s *statsTracker) StartService() (err error) {
	helloFn := func(write func(interface{})) {
		log.Debugf("Sending Lantern stats to new client")
		write(s.Latest())
	}

	s.service, err = ws.Register("stats", helloFn)
	return
}

func (s *statsTracker) StopService() {
	ws.Unregister("stats")
}
