package stats

import (
	"sync"
)

type Stats struct {
	City          string `json:"city"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	HTTPSUpgrades int    `json:"httpsUpgrades"`
	AdsBlocked    int    `json:"adsBlocked"`
}

type StatsTracker struct {
	mu        sync.Mutex
	stats     Stats
	Broadcast func(Stats)
}

func (s StatsTracker) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.stats.City, s.stats.Country, s.stats.CountryCode = city, country, countryCode
	s.unlockAndBroadcast()
}

func (s StatsTracker) IncHTTPSUpgrades() {
	s.mu.Lock()
	s.stats.HTTPSUpgrades++
	s.unlockAndBroadcast()
}

func (s StatsTracker) IncAdsBlocked() {
	s.mu.Lock()
	s.stats.AdsBlocked++
	s.unlockAndBroadcast()
}

func (s *StatsTracker) unlockAndBroadcast() {
	st := s.stats
	s.mu.Unlock()
	s.Broadcast(st)
}
