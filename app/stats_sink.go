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

type statsSink struct {
	mu      sync.Mutex
	service ws.Service
	stats   stats
}

func (s *statsSink) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.stats.City, s.stats.Country, s.stats.CountryCode = city, country, countryCode
	s.unlockAndBroadcast()
}
func (s *statsSink) IncHTTPSUpgrades() {
	s.mu.Lock()
	s.stats.HTTPSUpgrades++
	s.unlockAndBroadcast()
}
func (s *statsSink) IncAdsBlocked() {
	s.mu.Lock()
	s.stats.AdsBlocked++
	s.unlockAndBroadcast()
}

func (s *statsSink) unlockAndBroadcast() {
	st := s.stats
	s.mu.Unlock()
	s.service.Out <- st
}

func (s *statsSink) StartService() error {
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

func (s *statsSink) StopService() {
	ws.Unregister("stats")
}
