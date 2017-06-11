package android

import (
	"sync"
)

type stats struct {
	City          string `json:"city"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	HTTPSUpgrades int    `json:"httpsUpgrades"`
	AdsBlocked    int    `json:"adsBlocked"`
}

type statsTracker struct {
	mu    sync.Mutex
	stats stats
	user  UserConfig
}

func (s *statsTracker) Configure(user UserConfig) {

	s.mu.Lock()
	s.user = user
	s.mu.Unlock()
}

func (s statsTracker) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.stats.City, s.stats.Country, s.stats.CountryCode = city, country, countryCode
	s.unlockAndBroadcast()
}

func (s statsTracker) IncHTTPSUpgrades() {
	s.mu.Lock()
	s.stats.HTTPSUpgrades++
	s.unlockAndBroadcast()
}
func (s statsTracker) IncAdsBlocked() {
	s.mu.Lock()
	s.stats.AdsBlocked++
	s.unlockAndBroadcast()
}

func (s *statsTracker) unlockAndBroadcast() {
	st := s.stats
	s.mu.Unlock()
	s.user.UpdateStats(st.City, st.Country, st.CountryCode,
		st.HTTPSUpgrades,
		st.AdsBlocked)
}
