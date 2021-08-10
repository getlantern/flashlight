package geolookup

// Usage: It's always better to instantiate a new Geolookup instance for whatever you need.
// If that's not possible (e.g., because of legacy code), just use the singleton 'DefaultInstance'.
//
// Geolookup is already mocked in <geolookup/mocks/Geolookup.go>. See example
// usage in lantern-desktop's <desktop/replica/server_test.go:TestNewDynamicEndpointFromGeolocation>

import (
	"math"
	"sync"
	"time"

	"github.com/getlantern/eventual"
	geo "github.com/getlantern/geolookup"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	DefaultInstance Geolookup = NewGeolookup()
	log                       = golog.LoggerFor("flashlight.geolookup")
)

type Geolookup interface {
	GetIP(timeout time.Duration) string
	GetCountry(timeout time.Duration) string
	Refresh()
	OnRefresh() <-chan bool
}

type GeolookupInst struct {
	refreshRequest      chan interface{}
	watchers            []chan bool
	muWatchers          sync.RWMutex
	currentGeoInfo      eventual.Value
	waitForProxyTimeout time.Duration
	retryWaitMillis     int
	maxRetryWait        time.Duration
}

func NewGeolookup() *GeolookupInst {
	inst := &GeolookupInst{
		refreshRequest:      make(chan interface{}, 1),
		currentGeoInfo:      eventual.NewValue(),
		waitForProxyTimeout: 1 * time.Minute,
		retryWaitMillis:     100,
		maxRetryWait:        30 * time.Second,
	}
	go inst.setup()
	return inst
}

type geoInfo struct {
	ip   string
	city *geo.City
}

// GetIP gets the IP. If the IP hasn't been determined yet, waits up to the
// given timeout for an IP to become available.
func (self *GeolookupInst) GetIP(timeout time.Duration) string {
	gi, ok := self.currentGeoInfo.Get(timeout)
	if !ok || gi == nil {
		return ""
	}
	return gi.(*geoInfo).ip
}

// GetCountry gets the country. If the country hasn't been determined yet, waits
// up to the given timeout for a country to become available.
func (self *GeolookupInst) GetCountry(timeout time.Duration) string {
	gi, ok := self.currentGeoInfo.Get(timeout)
	if !ok || gi == nil {
		return ""
	}
	return gi.(*geoInfo).city.Country.IsoCode
}

// Refresh refreshes the geolookup information by calling the remote geolookup
// service. It will keep calling the service until it's able to determine an IP
// and country.
func (self *GeolookupInst) Refresh() {
	select {
	case self.refreshRequest <- true:
		log.Debug("Requested refresh")
	default:
		log.Debug("Refresh already in progress")
	}
}

// OnRefresh creates a channel that caller can receive on when new geolocation
// information is got.
func (self *GeolookupInst) OnRefresh() <-chan bool {
	ch := make(chan bool, 1)
	self.muWatchers.Lock()
	self.watchers = append(self.watchers, ch)
	self.muWatchers.Unlock()
	return ch
}

func (self *GeolookupInst) setup() {
	for _ = range self.refreshRequest {
		gi := self.lookup()
		if gi.ip == self.GetIP(0) {
			log.Debug("public IP doesn't change, not update")
			continue
		}
		self.currentGeoInfo.Set(gi)
		self.muWatchers.RLock()
		w := self.watchers
		self.muWatchers.RUnlock()
		for _, ch := range w {
			select {
			case ch <- true:
			default:
			}
		}
	}
}

func (self *GeolookupInst) lookup() *geoInfo {
	consecutiveFailures := 0

	for {
		gi, err := self.doLookup()
		if err != nil {
			log.Debugf("Unable to get current location: %s", err)
			wait := time.Duration(math.Pow(2, float64(consecutiveFailures))*float64(self.retryWaitMillis)) * time.Millisecond
			if wait > self.maxRetryWait {
				wait = self.maxRetryWait
			}
			log.Debugf("Waiting %v before retrying", wait)
			time.Sleep(wait)
			consecutiveFailures++
		} else {
			return gi
		}
	}
}

func (self *GeolookupInst) doLookup() (*geoInfo, error) {
	op := ops.Begin("geolookup")
	defer op.End()
	city, ip, err := geo.LookupIP("", proxied.ParallelPreferChained())

	if err != nil {
		log.Errorf("Could not lookup IP %v", err)
		return nil, op.FailIf(err)
	}
	return &geoInfo{ip, city}, nil
}
