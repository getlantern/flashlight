package analytics

import (
	"bytes"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/util"
	"github.com/kardianos/osext"

	log "github.com/sirupsen/logrus"
)

const (
	trackingID = "UA-21815217-12"

	// endpoint is the endpoint to report GA data to.
	endpoint = `https://ssl.google-analytics.com/collect`
)

var (
	

	maxWaitForIP = math.MaxInt32 * time.Second

	// Hash of the executable
	hash = getExecutableHash()
)

// Start starts the GA session with the given data.
func Start(deviceID, version string) func() {
	return start(deviceID, version, geolookup.GetIP, trackSession)
}

// start starts the GA session with the given data.
func start(deviceID, version string, ipFunc func(time.Duration) string,
	transport func(string)) func() {
	var addr atomic.Value
	go func() {
		ip := ipFunc(maxWaitForIP)
		if ip == "" {
			log.Errorf("No IP found within %v", maxWaitForIP)
		}
		addr.Store(ip)
		log.Infof("Starting analytics session with ip %v", ip)
		startSession(ip, version, deviceID, transport)
	}()

	stop := func() {
		if addr.Load() != nil {
			ip := addr.Load().(string)
			log.Infof("Ending analytics session with ip %v", ip)
			endSession(ip, version, deviceID, transport)
		}
	}
	return stop
}

func sessionVals(ip, version, clientID, sc string) string {
	vals := make(url.Values, 0)

	vals.Add("v", "1")
	vals.Add("cid", clientID)
	vals.Add("tid", trackingID)

	if ip != "" {
		// Override the users IP so we get accurate geo data.
		vals.Add("uip", ip)
	}

	// Make call to anonymize the user's IP address -- basically a policy thing where
	// Google agrees not to store it.
	vals.Add("aip", "1")

	vals.Add("dp", "localhost")
	vals.Add("t", "pageview")

	// Custom dimension for the Lantern version
	vals.Add("cd1", version)

	// Custom dimension for the hash of the executable. We combine the version
	// to make it easier to interpret in GA.
	vals.Add("cd2", version+"-"+hash)

	// This forces the recording of the session duration. It must be either
	// "start" or "end". See:
	// https://developers.google.com/analytics/devguides/collection/protocol/v1/parameters
	vals.Add("sc", sc)

	return vals.Encode()
}

// getExecutableHash returns the hash of the currently running executable.
// If there's an error getting the hash, this returns
func getExecutableHash() string {
	// We don't know how to get a useful hash here for Android but also this
	// code isn't currently called on Android, so just guard against Something
	// bad happening here.
	if runtime.GOOS == "android" {
		return "android"
	}
	if lanternPath, err := osext.Executable(); err != nil {
		log.Infof("Could not get path to executable %v", err)
		return err.Error()
	} else {
		if b, er := util.GetFileHash(lanternPath); er != nil {
			return er.Error()
		} else {
			return b
		}
	}
}

func endSession(ip string, version string,
	clientID string, transport func(string)) {
	args := sessionVals(ip, version, clientID, "end")
	transport(args)
}

func startSession(ip string, version string,
	clientID string, transport func(string)) {
	args := sessionVals(ip, version, clientID, "start")
	transport(args)
}

func trackSession(args string) {
	rt, err := proxied.ChainedNonPersistent("")
	if err != nil {
		log.Errorf("Could not create HTTP client: %s", err)
		return
	}
	doTrackSession(args, rt)
}

func doTrackSession(args string, rt http.RoundTripper) {
	r, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(args))

	if err != nil {
		log.Errorf("Error constructing GA request: %s", err)
		return
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Content-Length", strconv.Itoa(len(args)))

	if req, er := httputil.DumpRequestOut(r, true); er != nil {
		log.Infof("Could not dump request: %v", er)
	} else {
		log.Infof("Full analytics request: %v", string(req))
	}

	resp, err := rt.RoundTrip(r)
	if err != nil {
		log.Errorf("Could not send HTTP request to GA: %s", err)
		return
	}
	log.Infof("Successfully sent request to GA: %s", resp.Status)
	if err := resp.Body.Close(); err != nil {
		log.Infof("Unable to close response body: %v", err)
	}
}

// AddCampaign adds Google Analytics campaign tracking to a URL and returns
// that URL.
func AddCampaign(urlStr, campaign, content, medium string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		log.Errorf("Could not parse click URL: %v", err)
		return "", err
	}

	q := u.Query()
	q.Set("utm_source", runtime.GOOS)
	q.Set("utm_medium", medium)
	q.Set("utm_campaign", campaign)
	q.Set("utm_content", content)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
