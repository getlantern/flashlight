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
	updates chan Status
	mx      sync.RWMutex
}

func newStatus() *status {
	s := &status{
		Status: Status{
			On: true,
		},
		updates: make(chan Status, 1000)}
	pro.OnProStatusChange(func(isPro bool) {
		s.mx.Lock()
		s.IsPro = isPro
		s.mx.Unlock()
		s.dispatch()
	})
	settings.OnChange(SNOn, func(on interface{}) {
		s.mx.Lock()
		s.On = on.(bool)
		s.mx.Unlock()
		s.dispatch()
	})
	addDataCapListener(func(hitDataCap bool) {
		s.mx.Lock()
		s.HitDataCap = hitDataCap
		s.mx.Unlock()
		s.dispatch()
	})
	return s
}

func (s *status) dispatch() {
	s.mx.RLock()
	st := s.Status
	s.mx.RUnlock()
	select {
	case s.updates <- st:
		// okay
	default:
		// channel full
	}
	settings.setString(SNStatus, st.String())
}

func (s Status) String() string {
	if !s.On {
		return STATUS_DISCONNECTED
	} else if s.HitDataCap && !s.IsPro {
		return STATUS_THROTTLED
	} else if !s.HasSucceedingProxy {
		return STATUS_CONNECTING
	}
	return STATUS_CONNECTED
}
