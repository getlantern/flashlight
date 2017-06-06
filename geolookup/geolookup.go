package geolookup

import (
	"math"
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

	retryWaitMillis = 100
	maxRetryWait    = 30 * time.Second

	ServiceID  = service.ID("flashlight.geolookup")
	geoService *GeoLookup
)

type GeoInfo struct {
	ip   string
	city *geo.City
}

func (i *GeoInfo) GetIP() string {
	return i.ip
}

func (i *GeoInfo) GetCountry() string {
	return i.city.Country.IsoCode
}

// GeoLookup satisfies the service.Service interface
type GeoLookup struct {
	chStop           chan bool
	chRefreshRequest chan bool
	gi               eventual.Value
	p                service.Publisher
	service.Subscribable
}

func New() service.Subscribable {
	return &GeoLookup{
		chStop:           make(chan bool),
		chRefreshRequest: make(chan bool),
		gi:               eventual.NewValue(),
	}
}

func (s *GeoLookup) GetID() service.ID {
	return ServiceID
}

func (s *GeoLookup) SetPublisher(p service.Publisher) {
	s.p = p
}

func (s *GeoLookup) Start() {
	go s.loop()
}

func (s *GeoLookup) Stop() {
	s.chStop <- true
}

func (s *GeoLookup) Refresh() {
	select {
	case s.chRefreshRequest <- true:
	default:
		log.Debugf("geolookup in progress, skipping")
	}
}

func (s *GeoLookup) loop() {
	for {
		select {
		case <-s.chRefreshRequest:
			gi := lookup()
			current, ok := s.gi.Get(0)
			if !ok || current == nil {
			}
			if ok && current != nil && gi.ip == current.(*GeoInfo).ip {
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
	city, ip, err := geo.LookupIP("", proxied.ParallelPreferChained())

	if err != nil {
		log.Errorf("Could not lookup IP %v", err)
		return nil, op.FailIf(err)
	}
	return &GeoInfo{ip, city}, nil
}
