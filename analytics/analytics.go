package analytics

import (
	"bytes"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/util"
	"github.com/kardianos/osext"

	"github.com/getlantern/golog"
)

const (
	trackingID = "UA-21815217-12"

	// endpoint is the endpoint to report GA data to.
	endpoint = `https://ssl.google-analytics.com/collect`
)

var (
	log = golog.LoggerFor("flashlight.analytics")

	ServiceType service.Type = "flashlight.analytics"

	maxWaitForIP = math.MaxInt32 * time.Second
)

// analytics satisfies the service.Impl interface
type analytics struct {
	hash      string
	opts      *ConfigOpts
	transport func(string)
}

type ConfigOpts struct {
	DeviceID string
	Version  string
	IP       string
	Enabled  bool
}

func New() service.Impl {
	return &analytics{
		hash: getExecutableHash(),
	}
}

func (o *ConfigOpts) For() service.Type {
	return ServiceType
}

func (o *ConfigOpts) Complete() bool {
	return o.IP != ""
}

func (s *analytics) GetType() service.Type {
	return ServiceType
}

func (s *analytics) Reconfigure(_p service.Publisher, opts service.ConfigOpts) {
	s.opts = opts.(*ConfigOpts)
	s.transport = trackSession
}

func (s *analytics) Start() {
	if s.opts.Enabled {
		log.Debugf("Starting analytics session with ip %v", s.opts.IP)
		args := s.sessionVals("start")
		s.transport(args)
	}
}

func (s *analytics) Stop() {
	if s.opts.Enabled {
		log.Debugf("Ending analytics session with ip %v", s.opts.IP)
		args := s.sessionVals("end")
		s.transport(args)
	}
}

func (s *analytics) sessionVals(sc string) string {
	vals := make(url.Values, 0)

	vals.Add("v", "1")
	vals.Add("cid", s.opts.DeviceID)
	vals.Add("tid", trackingID)

	if s.opts.IP != "" {
		// Override the users IP so we get accurate geo data.
		vals.Add("uip", s.opts.IP)
	}

	// Make call to anonymize the user's IP address -- basically a policy thing where
	// Google agrees not to store it.
	vals.Add("aip", "1")

	vals.Add("dp", "localhost")
	vals.Add("t", "pageview")

	// Custom dimension for the Lantern version
	vals.Add("cd1", s.opts.Version)

	// Custom dimension for the hash of the executable. We combine the version
	// to make it easier to interpret in GA.
	vals.Add("cd2", s.opts.Version+"-"+s.hash)

	// This forces the recording of the session duration. It must be either
	// "start" or "end". See:
	// https://developers.google.com/analytics/devguides/collection/protocol/v1/parameters
	vals.Add("sc", sc)

	return vals.Encode()
}

// GetExecutableHash returns the hash of the currently running executable.
// If there's an error getting the hash, this returns
func getExecutableHash() string {
	// We don't know how to get a useful hash here for Android but also this
	// code isn't currently called on Android, so just guard against Something
	// bad happening here.
	if runtime.GOOS == "android" {
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

func trackSession(args string) {
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

	rt, err := proxied.ChainedNonPersistent("")
	if err != nil {
		log.Errorf("Could not create HTTP client: %s", err)
		return
	}
	resp, err := rt.RoundTrip(r)
	if err != nil {
		log.Errorf("Could not send HTTP request to GA: %s", err)
		return
	}
	log.Debugf("Successfully sent request to GA: %s", resp.Status)
	if err := resp.Body.Close(); err != nil {
		log.Debugf("Unable to close response body: %v", err)
	}
}
