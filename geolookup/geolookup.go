package geolookup

import (
	"math"
	"sync"
	"time"

	"github.com/getlantern/eventual"
	geo "github.com/getlantern/geolookup"
	"github.com/getlantern/zaplog"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	log = logging.LoggerFor("flashlight.geolookup")

	refreshRequest = make(chan interface{}, 1)
	currentGeoInfo = eventual.NewValue()
	watchers       []chan bool
	muWatchers     sync.RWMutex

	waitForProxyTimeout = 1 * time.Minute
	retryWaitMillis     = 100
	maxRetryWait        = 30 * time.Second
)

type geoInfo struct {
	ip   string
	city *geo.City
}

// GetIP gets the IP. If the IP hasn't been determined yet, waits up to the
// given timeout for an IP to become available.
func GetIP(timeout time.Duration) string {
	gi, ok := currentGeoInfo.Get(timeout)
	if !ok || gi == nil {
		return ""
	}
	return gi.(*geoInfo).ip
}

// GetCountry gets the country. If the country hasn't been determined yet, waits
// up to the given timeout for a country to become available.
func GetCountry(timeout time.Duration) string {
	gi, ok := currentGeoInfo.Get(timeout)
	if !ok || gi == nil {
		return ""
	}
	return gi.(*geoInfo).city.Country.IsoCode
}

// Refresh refreshes the geolookup information by calling the remote geolookup
// service. It will keep calling the service until it's able to determine an IP
// and country.
func Refresh() {
	select {
	case refreshRequest <- true:
		log.Info("Requested refresh")
	default:
		log.Info("Refresh already in progress")
	}
}

// OnRefresh creates a channel that caller can receive on when new geolocation
// information is got.
func OnRefresh() <-chan bool {
	ch := make(chan bool, 1)
	muWatchers.Lock()
	watchers = append(watchers, ch)
	muWatchers.Unlock()
	return ch
}

func init() {
	go run()
}

func run() {
	for _ = range refreshRequest {
		gi := lookup()
		if gi.ip == GetIP(0) {
			log.Info("public IP doesn't change, not update")
			continue
		}
		currentGeoInfo.Set(gi)
		muWatchers.RLock()
		w := watchers
		muWatchers.RUnlock()
		for _, ch := range w {
			select {
			case ch <- true:
			default:
			}
		}
	}
}

func lookup() *geoInfo {
	consecutiveFailures := 0

	for {
		gi, err := doLookup()
		if err != nil {
			log.Infof("Unable to get current location: %s", err)
			wait := time.Duration(math.Pow(2, float64(consecutiveFailures))*float64(retryWaitMillis)) * time.Millisecond
			if wait > maxRetryWait {
				wait = maxRetryWait
			}
			log.Infof("Waiting %v before retrying", wait)
			time.Sleep(wait)
			consecutiveFailures++
		} else {
			return gi
		}
	}
}

func doLookup() (*geoInfo, error) {
	op := ops.Begin("geolookup")
	defer op.End()
	city, ip, err := geo.LookupIP("", proxied.ParallelPreferChained())

	if err != nil {
		log.Errorf("Could not lookup IP %v", err)
		return nil, op.FailIf(err)
	}
	return &geoInfo{ip, city}, nil
}
