package stats

import (
	"sync"

	"github.com/getlantern/flashlight/ws"
)

type stats struct {
	City          string `json:"city"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	HTTPSUpgrades int    `json:"httpsUpgrades"`
	AdsBlocked    int    `json:"adsBlocked"`
}

// Tracker implements common.StatsTracker interface and publishes any changes
// to websocket.
type Tracker struct {
	mu      sync.Mutex
	service *ws.Service
	stats   stats
}

func (s *Tracker) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.stats.City, s.stats.Country, s.stats.CountryCode = city, country, countryCode
	s.unlockAndBroadcast()
}

func (s *Tracker) IncHTTPSUpgrades() {
	s.mu.Lock()
	s.stats.HTTPSUpgrades++
	s.unlockAndBroadcast()
}

func (s *Tracker) IncAdsBlocked() {
	s.mu.Lock()
	s.stats.AdsBlocked++
	s.unlockAndBroadcast()
}

func (s *Tracker) unlockAndBroadcast() {
	st := s.stats
	s.mu.Unlock()
	if s.service != nil {
		select {
		case s.service.Out <- st:
			// ok
		default:
			// don't block if no-one is listening
		}
	}
}

func (s *Tracker) StartUIService() (err error) {
	helloFn := func(write func(interface{})) {
		s.mu.Lock()
		st := s.stats
		s.mu.Unlock()
		write(st)
	}

	s.service, err = ws.Register("stats", helloFn)
	return
}

func (s *Tracker) StopUIService() {
	ws.Unregister("stats")
}
