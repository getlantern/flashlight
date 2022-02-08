package geolookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/getlantern/eventual/v2"
	geo "github.com/getlantern/geolookup"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	log = golog.LoggerFor("flashlight.geolookup")

	refreshRequest = make(chan interface{}, 1)
	currentGeoInfo = eventual.NewValue()
	watchers       []chan bool
	persistToFile  string
	mx             sync.Mutex
	roundTripper   http.RoundTripper = proxied.ParallelPreferChained()
)

const (
	maxTimeout      = 10 * time.Minute
	retryWaitMillis = 100
	maxRetryWait    = 30 * time.Second
)

type GeoInfo struct {
	IP   string
	City *geo.City
}

// GetIP gets the IP. If the IP hasn't been determined yet, waits up to the
// given timeout for an IP to become available.
func GetIP(timeout time.Duration) string {
	gi, err := getGeoInfo(timeout)
	if err != nil {
		log.Debugf("Could not get IP: %v", err)
		return ""
	}
	return gi.IP
}

// GetCountry gets the country. If the country hasn't been determined yet, waits
// up to the given timeout for a country to become available.
func GetCountry(timeout time.Duration) string {
	gi, err := getGeoInfo(timeout)
	if err != nil {
		log.Debugf("Could not get country: %v", err)
		return ""
	}
	return gi.City.Country.IsoCode
}

func getGeoInfo(timeout time.Duration) (*GeoInfo, error) {
	if timeout < 0 {
		timeout = maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	gi, err := currentGeoInfo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get geoinfo with timeout %v: %w", timeout, err)
	}
	if gi == nil {
		return nil, fmt.Errorf("no geo info after %v", timeout)
	}
	return gi.(*GeoInfo), nil
}

// EnablePersistence enables persistence of the current geo info to disk at the named file and
// initializes current geo info from that file if necessary.
func EnablePersistence(geoFile string) {
	mx.Lock()
	defer mx.Unlock()

	// use this file going forward
	persistToFile = geoFile

	log.Debugf("Will persist geolocation info to %v", persistToFile)

	// initialize from file if necessary
	knownCountry := GetCountry(0)
	if knownCountry == "" {
		file, err := os.Open(persistToFile)
		if err == nil {
			log.Debugf("Initializing geolocation info from %v", persistToFile)
			dec := json.NewDecoder(file)
			gi := &GeoInfo{}
			decodeErr := dec.Decode(gi)
			if decodeErr != nil {
				log.Errorf("Error initializing geolocation info from %v: %v", persistToFile, decodeErr)
				return
			}
			setGeoInfo(gi, false)
		}
	}
}

// Refresh refreshes the geolookup information by calling the remote geolookup
// service. It will keep calling the service until it's able to determine an IP
// and country.
func Refresh() {
	select {
	case refreshRequest <- true:
		log.Debug("Requested refresh")
	default:
		log.Debug("Refresh already in progress")
	}
}

// OnRefresh creates a channel that caller can receive on when new geolocation
// information is got.
func OnRefresh() <-chan bool {
	ch := make(chan bool, 1)
	mx.Lock()
	watchers = append(watchers, ch)
	mx.Unlock()
	return ch
}

func init() {
	go run()
}

func run() {
	for range refreshRequest {
		gi := lookup()
		if gi.IP == GetIP(0) {
			log.Debug("public IP did not change - not notifying watchers")
			continue
		}
		mx.Lock()
		setGeoInfo(gi, true)
		mx.Unlock()
	}
}

func setGeoInfo(gi *GeoInfo, persist bool) {
	currentGeoInfo.Set(gi)
	w := watchers
	for _, ch := range w {
		select {
		case ch <- true:
		default:
		}
	}
	if persist && persistToFile != "" {
		b, err := json.Marshal(gi)
		if err != nil {
			log.Errorf("Unable to marshal geolocation info to JSON for persisting: %v", err)
			return
		}
		writeErr := ioutil.WriteFile(persistToFile, b, 0644)
		if writeErr != nil {
			log.Errorf("Error persisting geolocation info to %v: %v", persistToFile, err)
		}
	}
}

func lookup() *GeoInfo {
	consecutiveFailures := 0

	for {
		gi, err := doLookup()
		if err != nil {
			log.Debugf("Unable to get current location: %s", err)
			wait := time.Duration(math.Pow(2, float64(consecutiveFailures))*float64(retryWaitMillis)) * time.Millisecond
			if wait > maxRetryWait {
				wait = maxRetryWait
			}
			log.Debugf("Waiting %v before retrying", wait)
			time.Sleep(wait)
			consecutiveFailures++
		} else {
			return gi
		}
	}
}

func doLookup() (*GeoInfo, error) {
	op := ops.Begin("geolookup")
	defer op.End()
	city, ip, err := geo.LookupIP("", roundTripper)

	if err != nil {
		log.Errorf("Could not lookup IP %v", err)
		return nil, op.FailIf(err)
	}
	return &GeoInfo{ip, city}, nil
}
