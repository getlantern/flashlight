package analytics

import (
	"bytes"
	"github.com/getlantern/flashlight/analytics/engine"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/kardianos/osext"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/util"
)


var (
	log = golog.LoggerFor("flashlight.analytics")

	// GA and Matomo ends a session after 30 minutes of inactivity. Prevent it by sending
	// keepalive. Note that the session still ends at the midnight.  See
	// https://support.google.com/analytics/answer/2731565?hl=en
	// https://help.piwik.pro/support/questions/how-is-a-session-counted-in-piwik-pro/
	keepaliveInterval = 25 * time.Minute
)

// Start starts the analytics session with the given data.
func Start(deviceID, version string) *session {
	s := newSession(deviceID, version, keepaliveInterval, proxied.ChainedThenFronted())
	go s.keepalive()
	return s
}

type Session interface {
	SetIP(string)
	EventWithLabel(string, string, string)
	Event(string, string)
	End()
}

type NullSession struct{}

func (s NullSession) SetIP(string)                          {}
func (s NullSession) EventWithLabel(string, string, string) {}
func (s NullSession) Event(string, string)                  {}
func (s NullSession) End()                                  {}

type session struct {
	vals              url.Values
	muVals            sync.RWMutex
	rt                http.RoundTripper
	keepaliveInterval time.Duration
	chDoneTracking    chan struct{}
	engine engine.Engine
}

func newSession(deviceID, version string, keepalive time.Duration, rt http.RoundTripper) *session {
	eng := engine.New()
	return &session{
		vals:              eng.GetSessionValues(version, deviceID, getExecutableHash()),
		rt:                rt,
		keepaliveInterval: keepalive,
		chDoneTracking:    make(chan struct{}),
		engine: eng,
	}
}

// SetIP sets the client IP for better analysis. The IP is always anonymized by
// both Google and matomo engines
func (s *session) SetIP(ip string) {
	s.muVals.Lock()
	s.vals = s.engine.SetIP(s.vals, ip)
	s.muVals.Unlock()
	go s.track()
}

// EventWithLabel tells engine that some event happens in the current page with a label
func (s *session) EventWithLabel(category, action, label string) {
	s.muVals.Lock()
	s.vals = s.engine.SetEventWithLabel(s.vals, category, action, label)
	s.muVals.Unlock()
	go s.track()
}

// Event tells engine that some event happens in the current page.
func (s *session) Event(category, action string) {
	s.EventWithLabel(category, action, "")
}

// End tells engine to force end the current session.
func (s *session) End() {
	s.muVals.Lock()
	s.vals = s.engine.End(s.vals)
	s.muVals.Unlock()
	go s.track()
}

// keepalive keeps tracking session with the latest parameters to avoid engine from
// ending the session.
func (s *session) keepalive() {
	keepaliveTimer := time.NewTimer(s.keepaliveInterval)
	for {
		select {
		case <-s.chDoneTracking:
			// Note this does not drain the channel before resetting, so
			// keepalive may be sent more than required, but that is okay.
			keepaliveTimer.Reset(s.keepaliveInterval)
		case <-keepaliveTimer.C:
			go s.track()
		}
	}
}

func (s *session) track() {
	s.muVals.RLock()
	args := s.vals.Encode()
	s.muVals.RUnlock()
	defer func() {
		select {
		case s.chDoneTracking <- struct{}{}:
		default:
			// tests may not have keepalive loop to receive from the channel
		}
	}()

	r, err := http.NewRequest("POST", s.engine.GetEndpoint(), bytes.NewBufferString(args))
	if err != nil {
		_ = log.Errorf("Error constructing analytics request: %s", err)
		return
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Content-Length", strconv.Itoa(len(args)))

	if req, er := httputil.DumpRequestOut(r, true); er != nil {
		log.Debugf("Could not dump request: %v", er)
	} else {
		log.Debugf("Full analytics request: %v", string(req))
	}

	resp, err := s.rt.RoundTrip(r)
	if err != nil {
		_ = log.Errorf("Could not send HTTP request to analytics: %s", err)
		return
	}
	log.Debugf("Successfully sent request to analytics: %s", resp.Status)
	if err := resp.Body.Close(); err != nil {
		log.Debugf("Unable to close response body: %v", err)
	}
}

// getExecutableHash returns the hash of the currently running executable.
// If there's an error getting the hash, this returns
func getExecutableHash() string {
	// We don't know how to get a useful hash here for Android but also this
	// code isn't currently called on Android, so just guard against something
	// bad happening here.
	if common.Platform == "android" {
		return "android"
	}
	if lanternPath, err := osext.Executable(); err != nil {
		log.Debugf("Could not get path to executable %v", err)
		return err.Error()
	} else {
		if b, er := util.GetFileHash(lanternPath); er != nil {
			return er.Error()
		} else {
			return b
		}
	}
}
