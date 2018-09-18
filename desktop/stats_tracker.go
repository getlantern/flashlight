package desktop

import (
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ws"
)

type statsTracker struct {
	stats.Tracker
	service *ws.Service
}

func NewStatsTracker() *statsTracker {
	return &statsTracker{
		Tracker: stats.NewTracker(),
	}
}

func (s *statsTracker) StartService(channel ws.UIChannel) (err error) {
	helloFn := func(write func(interface{})) {
		log.Infof("Sending Lantern stats to new client")
		write(s.Latest())
	}

	s.service, err = channel.Register("stats", helloFn)
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
