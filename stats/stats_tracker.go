package stats

import (
	"reflect"
	"sync"
	"time"

	"github.com/getlantern/event"
	"github.com/getlantern/zaplog"
)

const (
	STATUS_CONNECTING   = "connecting"
	STATUS_CONNECTED    = "connected"
	STATUS_DISCONNECTED = "disconnected"
	STATUS_THROTTLED    = "throttled"
)

type AlertType string

const (
	FAIL_TO_SET_SYSTEM_PROXY AlertType = "fail_to_set_system_proxy"
	FAIL_TO_OPEN_BROWSER     AlertType = "fail_to_open_browser"
	// TODO: code to trigger this alert
	NO_INTERNET_CONNECTION AlertType = "no_internet_connection"
)

var (
	log = zaplog.LoggerFor("flashlight.stats")
)

type Alert struct {
	AlertType AlertType `json:"alertType"`
	Details   string    `json:"alertDetails"`
	HelpURL   string    `json:"helpURL"`
}

func (e Alert) Alert() string {
	return string(e.AlertType)
}

var helpURLs = map[AlertType]string{
	FAIL_TO_SET_SYSTEM_PROXY: "https://github.com/getlantern/lantern/wiki/Troubleshooting:-Failed-to-set-Lantern-as-system-proxy",
	FAIL_TO_OPEN_BROWSER:     "https://github.com/getlantern/lantern/wiki/Troubleshooting:-Failed-to-open-browser-window-to-show-the-Lantern-user-interface",
	NO_INTERNET_CONNECTION:   "",
}

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

	// SetAlert indicates that some alert needs user attention. If transient is
	// true, the alert will be cleared automatically 10 seconds later.
	SetAlert(alertType AlertType, details string, transient bool)

	// ClearAlert clears the alert state if the current alert has the specific
	// type.
	ClearAlert(alertType AlertType)
}

// Stats are stats and status of the current Lantern
type Stats struct {
	City               string  `json:"city"`
	Country            string  `json:"country"`
	CountryCode        string  `json:"countryCode"`
	HTTPSUpgrades      int     `json:"httpsUpgrades"`
	AdsBlocked         int     `json:"adsBlocked"`
	Disconnected       bool    `json:"disconnected"`
	HasSucceedingProxy bool    `json:"hasSucceedingProxy"`
	HitDataCap         bool    `json:"hitDataCap"`
	IsPro              bool    `json:"isPro"`
	Status             string  `json:"status"`
	Alerts             []Alert `json:"alerts"`
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
		dispatcher: event.NewDispatcher(true, 1000),
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

func (t *tracker) SetAlert(alertType AlertType, details string, transient bool) {
	t.update(func(stats Stats) Stats {
		log.Infof("Setting alert %s", alertType)
		e := Alert{alertType, details, helpURLs[alertType]}
		stats.Alerts = append(stats.Alerts, e)
		if transient {
			go func() {
				time.Sleep(10 * time.Second)
				t.ClearAlert(alertType)
			}()
		}
		return stats
	})
}

func (t *tracker) ClearAlert(alertType AlertType) {
	t.update(func(stats Stats) Stats {
		alerts := stats.Alerts[:0]
		for _, a := range stats.Alerts {
			if a.AlertType == alertType {
				log.Infof("Clearing alert %s", alertType)
			} else {
				alerts = append(alerts, a)
			}
		}
		stats.Alerts = alerts
		return stats
	})
}

func (t *tracker) update(update func(stats Stats) Stats) {
	t.mx.Lock()
	stats := update(t.stats)
	if !reflect.DeepEqual(stats, t.stats) {
		if stats.Disconnected {
			stats.Status = STATUS_DISCONNECTED
		} else if !stats.HasSucceedingProxy {
			stats.Status = STATUS_CONNECTING
		} else {
			stats.Status = STATUS_CONNECTED
		}
		t.stats = stats
		// copy the slice to avoid data race between updating and consuming alerts
		t.stats.Alerts = make([]Alert, len(stats.Alerts))
		copy(t.stats.Alerts, stats.Alerts)
		t.dispatcher.Dispatch(stats)
	}
	t.mx.Unlock()
}
