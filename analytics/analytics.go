package analytics

import (
	"bytes"
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

const (
	trackingID = "UA-21815217-12"

	// endpoint is the endpoint to report GA data to.
	endpoint = `https://ssl.google-analytics.com/collect`
)

var (
	log = golog.LoggerFor("flashlight.analytics")

	// GA ends an session after 30 minutes of inactivity. Prevent it by sending
	// keepalive. Note that the session still ends at the midnight.  See
	// https://support.google.com/analytics/answer/2731565?hl=en
	keepaliveInterval = 25 * time.Minute
)

// Start starts the GA session with the given data.
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
}

func newSession(deviceID, version string, keepalive time.Duration, rt http.RoundTripper) *session {
	return &session{
		vals:              sessionVals(version, deviceID),
		rt:                rt,
		keepaliveInterval: keepalive,
		chDoneTracking:    make(chan struct{}),
	}
}

// SetIP sets the client IP for better analysis. The IP is always anonymized by
// Google.
func (s *session) SetIP(ip string) {
	s.muVals.Lock()
	s.vals.Set("uip", ip)
	s.muVals.Unlock()
	go s.track()
}

// EventWithLabel tells GA that some event happens in the current page with a label
func (s *session) EventWithLabel(category, action, label string) {
	s.muVals.Lock()
	s.vals.Set("ec", category)
	s.vals.Set("ea", action)
	s.vals.Set("el", label)
	s.vals.Set("t", "event")
	s.muVals.Unlock()
	go s.track()
}

// Event tells GA that some event happens in the current page.
func (s *session) Event(category, action string) {
	s.EventWithLabel(category, action, "")
}

// End tells GA to force end the current session.
func (s *session) End() {
	s.muVals.Lock()
	s.vals.Add("sc", "end")
	s.muVals.Unlock()
	go s.track()
}

// keepalive keeps tracking session with the latest parameters to avoid GA from
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

	r, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(args))

	if err != nil {
		log.Errorf("Error constructing GA request: %s", err)
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
		log.Errorf("Could not send HTTP request to GA: %s", err)
		return
	}
	log.Debugf("Successfully sent request to GA: %s", resp.Status)
	if err := resp.Body.Close(); err != nil {
		log.Debugf("Unable to close response body: %v", err)
	}
}

func sessionVals(version, clientID string) url.Values {
	vals := make(url.Values, 0)

	vals.Add("v", "1")
	vals.Add("cid", clientID)
	vals.Add("tid", trackingID)

	// Make call to anonymize the user's IP address -- basically a policy thing
	// where Google agrees not to store it.
	vals.Add("aip", "1")

	// Custom dimension for the Lantern version
	vals.Add("cd1", version)

	// Custom dimension for the hash of the executable. We combine the version
	// to make it easier to interpret in GA.
	vals.Add("cd2", version+"-"+getExecutableHash())

	vals.Add("dp", "localhost")
	vals.Add("t", "pageview")

	return vals
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
