package app

import (
	"sync"

	"github.com/getlantern/flashlight/pro"
)

const (
	STATUS_CONNECTING   = "connecting"
	STATUS_CONNECTED    = "connected"
	STATUS_DISCONNECTED = "disconnected"
	STATUS_THROTTLED    = "throttled"
)

// Status indicates the overall state of the app
type Status struct {
	On                 bool
	HasSucceedingProxy bool
	HitDataCap         bool
	IsPro              bool
}

// StatusUpdates obtains a channel from which one can read status updates for
// the app.
func (app *App) StatusUpdates() <-chan Status {
	return app.status.updates
}

type status struct {
	Status
	changes chan Status
	updates chan Status
	mx      sync.Mutex
}

func newStatus() *status {
	s := &status{
		Status: Status{
			On: true,
		},
		changes: make(chan Status, 1000),
		updates: make(chan Status, 1000),
	}

	pro.OnProStatusChange(func(isPro bool) {
		s.update(func(s Status) Status {
			s.IsPro = isPro
			return s
		})
	})
	settings.OnChange(SNOn, func(on interface{}) {
		s.update(func(s Status) Status {
			s.On = on.(bool)
			return s
		})
	})
	addDataCapListener(func(hitDataCap bool) {
		s.update(func(s Status) Status {
			s.HitDataCap = hitDataCap
			return s
		})
	})

	go s.dispatchChanges()
	return s
}

func (s *status) update(change func(Status) Status) {
	s.mx.Lock()
	s.Status = change(s.Status)
	select {
	case s.changes <- s.Status:
		// okay
	default:
		// channel full
	}
	s.mx.Unlock()
}

func (s *status) dispatchChanges() {
	var prior Status
	for st := range s.changes {
		if st != prior {
			select {
			case s.updates <- st:
				// okay
				prior = st
			default:
				// channel full
			}
			settings.setString(SNStatus, st.String())
		}
	}
}

func (s Status) String() string {
	if !s.On {
		return STATUS_DISCONNECTED
	} else if !s.HasSucceedingProxy {
		return STATUS_CONNECTING
	}
	return STATUS_CONNECTED
}
