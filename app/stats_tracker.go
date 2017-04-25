package app

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

type statsTracker struct {
	mu      sync.Mutex
	service ws.Service
	stats   stats
}

func (s *statsTracker) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.stats.City, s.stats.Country, s.stats.CountryCode = city, country, countryCode
	s.unlockAndBroadcast()
}

func (s *statsTracker) IncHTTPSUpgrades() {
	s.mu.Lock()
	s.stats.HTTPSUpgrades++
	s.unlockAndBroadcast()
}

func (s *statsTracker) IncAdsBlocked() {
	s.mu.Lock()
	s.stats.AdsBlocked++
	s.unlockAndBroadcast()
}

func (s *statsTracker) unlockAndBroadcast() {
	st := s.stats
	s.mu.Unlock()
	select {
	case s.service.Out <- st:
		// ok
	default:
		// don't block if no-one is listening
	}
}

func (s *statsTracker) StartService() error {
	helloFn := func(write func(interface{})) {
		log.Debugf("Sending Lantern stats to new client")
		s.mu.Lock()
		st := s.stats
		s.mu.Unlock()
		write(st)
	}

	_, err := ws.Register("stats", helloFn)
	return err
}

func (s *statsTracker) StopService() {
	ws.Unregister("stats")
}
