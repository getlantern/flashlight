package android

import (
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/stats"
)

type statsTracker struct {
	stats.Tracker
	session pro.Session
}

func NewStatsTracker(session pro.Session) *statsTracker {
	s := &statsTracker{
		session: session,
	}
	s.Broadcast = func(st stats.Stats) {
		s.session.UpdateStats(st.City, st.Country,
			st.CountryCode, st.HTTPSUpgrades, st.AdsBlocked)
	}
	return s
}
