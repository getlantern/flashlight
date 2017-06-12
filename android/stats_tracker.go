package android

import (
	"github.com/getlantern/flashlight/stats"
)

type statsTracker struct {
	stats.Tracker
	user UserConfig
}

func (s *statsTracker) Configure(user UserConfig) {
	s.user = user
	s.Broadcast = func(st stats.Stats) {
		s.user.UpdateStats(st.City, st.Country,
			st.CountryCode, st.HTTPSUpgrades, st.AdsBlocked)
	}
}
