package stats

import (
	"sync"
)

// StatsTracker is a common interface to receive user perceptible Lantern stats
type StatsTracker interface {
	// SetActiveProxyLocation updates the location of last successfully dialed
	// proxy. countryCode is in ISO Alpha-2 form, see
	// http://www.nationsonline.org/oneworld/country_code_list.htm
	SetActiveProxyLocation(city, country, countryCode string)
	// IncHTTPSUpgrades indicates that Lantern client redirects a HTTP request
	// to HTTPS via HTTPSEverywhere.
	IncHTTPSUpgrades()
	// IncAdsBlocked indicates that a proxy request is blocked per easylist rules.
	IncAdsBlocked()
}

type Stats struct {
	City          string `json:"city"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	HTTPSUpgrades int    `json:"httpsUpgrades"`
	AdsBlocked    int    `json:"adsBlocked"`
}

type Tracker struct {
	mu        sync.Mutex
	stats     Stats
	Broadcast func(Stats)
}

func (s Tracker) Latest() Stats {
	s.mu.Lock()
	st := s.stats
	s.mu.Unlock()
	return st
}

func (s Tracker) SetActiveProxyLocation(city, country, countryCode string) {
	s.mu.Lock()
	s.stats.City, s.stats.Country, s.stats.CountryCode = city, country, countryCode
	s.unlockAndBroadcast()
}

func (s Tracker) IncHTTPSUpgrades() {
	s.mu.Lock()
	s.stats.HTTPSUpgrades++
	s.unlockAndBroadcast()
}

func (s Tracker) IncAdsBlocked() {
	s.mu.Lock()
	s.stats.AdsBlocked++
	s.unlockAndBroadcast()
}

func (s *Tracker) unlockAndBroadcast() {
	st := s.stats
	s.mu.Unlock()
	if s.Broadcast != nil {
		s.Broadcast(st)
	}
}
