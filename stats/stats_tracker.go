package stats

import (
	"sync"

	"github.com/getlantern/event"
)

const (
	STATUS_CONNECTING   = "connecting"
	STATUS_CONNECTED    = "connected"
	STATUS_DISCONNECTED = "disconnected"
	STATUS_THROTTLED    = "throttled"
)

// Tracker is a common interface to receive user perceptible Lantern stats
type Tracker interface {
	// Latest returns the latest Stats that are being tracked
	Latest() Stats

	// AddListener registers a new listener for stats updates and returns a
	// function that can be used to close the listener.
	AddListener(func(newStats Stats)) (close func())

	// SetActiveProxyLocation updates the location of last successfully dialed
	// proxy. countryCode is in ISO Alpha-2 form, see
	// http://www.nationsonline.org/oneworld/country_code_list.htm
	SetActiveProxyLocation(city, country, countryCode string)

	// IncHTTPSUpgrades indicates that Lantern client redirects a HTTP request
	// to HTTPS via HTTPSEverywhere.
	IncHTTPSUpgrades()

	// IncAdsBlocked indicates that a proxy request is blocked per easylist rules.
	IncAdsBlocked()

	// SetDisconnected indicates that we've entered disconnected mode
	SetDisconnected(val bool)

	// SetHasSucceedingProxy indicates that we do (or don't) have a succeeding
	// proxy.
	SetHasSucceedingProxy(val bool)

	// SetHitDataCap indicates that we've hit the data cap
	SetHitDataCap(val bool)

	// SetIsPro indicates that we're pro
	SetIsPro(val bool)
}

// Stats are stats and status of the current Lantern
type Stats struct {
	City               string `json:"city"`
	Country            string `json:"country"`
	CountryCode        string `json:"countryCode"`
	HTTPSUpgrades      int    `json:"httpsUpgrades"`
	AdsBlocked         int    `json:"adsBlocked"`
	Disconnected       bool   `json:"disconnected"`
	HasSucceedingProxy bool   `json:"hasSucceedingProxy"`
	HitDataCap         bool   `json:"hitDataCap"`
	IsPro              bool   `json:"isPro"`
	Status             string `json:"status"`
}

// tracker is an implementation of Tracker which broadcasts changes as they
// happen.
type tracker struct {
	stats      Stats
	mx         sync.RWMutex
	dispatcher event.Dispatcher
}

// NewTracker creates a new Tracker
func NewTracker() Tracker {
	return &tracker{
		dispatcher: event.NewDispatcher(false, 1000),
	}
}

func (t *tracker) Latest() Stats {
	t.mx.RLock()
	stats := t.stats
	t.mx.RUnlock()
	return stats
}

func (t *tracker) AddListener(fn func(newStats Stats)) (close func()) {
	return t.dispatcher.AddListener(func(msg interface{}) {
		fn(msg.(Stats))
	})
}

func (t *tracker) SetActiveProxyLocation(city, country, countryCode string) {
	t.update(func(stats Stats) Stats {
		stats.City, stats.Country, stats.CountryCode = city, country, countryCode
		return stats
	})
}

func (t *tracker) IncHTTPSUpgrades() {
	t.update(func(stats Stats) Stats {
		stats.HTTPSUpgrades++
		return stats
	})
}

func (t *tracker) IncAdsBlocked() {
	t.update(func(stats Stats) Stats {
		stats.AdsBlocked++
		return stats
	})
}

func (t *tracker) SetDisconnected(val bool) {
	t.update(func(stats Stats) Stats {
		stats.Disconnected = val
		return stats
	})
}

func (t *tracker) SetHasSucceedingProxy(val bool) {
	t.update(func(stats Stats) Stats {
		stats.HasSucceedingProxy = val
		return stats
	})
}

func (t *tracker) SetHitDataCap(val bool) {
	t.update(func(stats Stats) Stats {
		stats.HitDataCap = val
		return stats
	})
}

func (t *tracker) SetIsPro(val bool) {
	t.update(func(stats Stats) Stats {
		stats.IsPro = val
		return stats
	})
}

func (t *tracker) update(update func(stats Stats) Stats) {
	t.mx.Lock()
	stats := update(t.stats)
	if stats != t.stats {
		if stats.Disconnected {
			stats.Status = STATUS_DISCONNECTED
		} else if !stats.HasSucceedingProxy {
			stats.Status = STATUS_CONNECTING
		} else {
			stats.Status = STATUS_CONNECTED
		}
		t.stats = stats
		t.dispatcher.Dispatch(stats)
	}
	t.mx.Unlock()
}
