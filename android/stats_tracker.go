package android

import (
	"sync"
)

type statsTracker struct {
	mu   sync.Mutex
	user UserConfig
}

func (s *statsTracker) Configure(user UserConfig) {

	s.mu.Lock()
	s.user = user
	s.mu.Unlock()
}

func (s statsTracker) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.user.SetProxyLocation(city, country, countryCode)
	s.mu.Unlock()
}
func (s statsTracker) IncHTTPSUpgrades() {}
func (s statsTracker) IncAdsBlocked()    {}
