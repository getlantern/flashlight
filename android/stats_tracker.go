package android

import (
	"github.com/getlantern/flashlight/stats"
)

type statsTracker struct {
	stats.Tracker
	user UserConfig
}

func NewStatsTracker(user UserConfig) *statsTracker {
	s := &statsTracker{
		user: user,
	}
	s.Broadcast = func(st stats.Stats) {
		s.user.UpdateStats(st.City, st.Country,
			st.CountryCode, st.HTTPSUpgrades, st.AdsBlocked)
	}
	return s
}
