package app

import (
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ws"
)

type statsTracker struct {
	stats.Tracker
	service *ws.Service
	close   func()
}

func NewStatsTracker() *statsTracker {
	return &statsTracker{
		Tracker: stats.NewTracker(),
	}
}

func (s *statsTracker) StartService() (err error) {
	helloFn := func(write func(interface{})) {
		log.Debugf("Sending Lantern stats to new client")
		write(s.Latest())
	}

	s.service, err = ws.Register("stats", helloFn)
	if err == nil {
		s.AddListener(func(newStats stats.Stats) {
			select {
			case s.service.Out <- newStats:
				// ok
			default:
				// don't block if no-one is listening
			}
		})
	}
	return
}

func (s *statsTracker) StopService() {
	ws.Unregister("stats")
	s.close()
}
