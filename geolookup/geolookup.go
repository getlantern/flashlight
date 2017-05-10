package geolookup

import (
	"math"
	"sync"
	"time"

	"github.com/getlantern/eventual"
	geo "github.com/getlantern/geolookup"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/service"
)

var (
	log = golog.LoggerFor("flashlight.geolookup")

	watchers   []chan bool
	muWatchers sync.RWMutex

	retryWaitMillis = 100
	maxRetryWait    = 30 * time.Second

	ServiceType = service.Type("flashlight.geolookup")
	geoService  *GeoLookup
)

type geoInfo struct {
	ip   string
	city *geo.City
}

func (i *geoInfo) ValidMessageFrom(t service.Type) bool {
	return t == ServiceType
}

// GeoLookup satisfies the service.Impl interface
type GeoLookup struct {
	chStop           chan bool
	chRefreshRequest chan bool
	gi               eventual.Value
	p                service.Publisher
}

func New() service.Impl {
	return &GeoLookup{
		chStop:           make(chan bool),
		chRefreshRequest: make(chan bool, 1),
		gi:               eventual.NewValue(),
	}
}

func (s *GeoLookup) GetType() service.Type {
	return ServiceType
}

func (s *GeoLookup) Reconfigure(p service.Publisher, opts service.ConfigOpts) {
	s.p = p
}

func (s *GeoLookup) Start() {
	log.Debugf("Starting geolookup service")
	go s.loop()
	s.chRefreshRequest <- true
	log.Debugf("Started geolookup service")
}

func (s *GeoLookup) Stop() {
	log.Debugf("Stopping geolookup service")
	s.chStop <- true
	log.Debugf("Stopped geolookup service")
}

// GetIP gets the IP. If the IP hasn't been determined yet, waits up to the
// given timeout for an IP to become available.
func (s *GeoLookup) GetIP(timeout time.Duration) string {
	gi, ok := s.gi.Get(timeout)
	if !ok || gi == nil {
		return ""
	}
	return gi.(*geoInfo).ip
}

// GetCountry gets the country. If the country hasn't been determined yet, waits
// up to the given timeout for a country to become available.
func (s *GeoLookup) GetCountry(timeout time.Duration) string {
	gi, ok := s.gi.Get(timeout)
	if !ok || gi == nil {
		return ""
	}
	return gi.(*geoInfo).city.Country.IsoCode
}

func (s *GeoLookup) loop() {
	for {
		select {
		case <-s.chRefreshRequest:
			gi := lookup()
			current, ok := s.gi.Get(0)
			if !ok || current == nil {
			}
			if ok && current != nil && gi.ip == current.(*geoInfo).ip {
				log.Debug("public IP doesn't change, not update")
				continue
			}
			s.gi.Set(gi)
			s.p.Publish(gi)
		case <-s.chStop:
			return
		}
	}
}

func lookup() *geoInfo {
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
