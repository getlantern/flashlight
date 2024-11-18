package geolookup

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"

	userconfig "github.com/getlantern/flashlight/v7/config/user"
)

var (
	log = golog.LoggerFor("flashlight.geolookup")

	watchers []chan bool
	mx       sync.Mutex

	setInitialValues = atomic.Bool{}
)

func init() {
	userconfig.OnConfigChange(func(old, new *userconfig.UserConfig) {
		setInitialValues.CompareAndSwap(false, true)

		// if the country or IP has changed, notify watchers
		if old == nil || old.Country != new.Country || old.Ip != new.Ip {
			log.Debugf("Country or IP changed, %v, %v. Notifying watchers", new.Country, new.Ip)
			mx.Lock()
			for _, ch := range watchers {
				select {
				case ch <- true:
				default:
				}
			}
			mx.Unlock()
		}
	})
}

// GetIP gets the IP. If the IP hasn't been determined yet, waits up to the given timeout for the
// IP to become available.
func GetIP(timeout time.Duration) string {
	conf, err := getConfig(timeout)
	if err != nil {
		if !setInitialValues.Load() {
			log.Debugf("IP not available yet")
		} else {
			log.Errorf("Failed to get IP: %v", err)
		}
		return ""
	}

	return conf.Ip
}

// GetCountry gets the country. If the country hasn't been determined yet, waits up to the given
// timeout for country to become available.
func GetCountry(timeout time.Duration) string {
	conf, err := getConfig(timeout)
	if err != nil {
		if !setInitialValues.Load() {
			log.Debugf("Country not available yet")
		} else {
			log.Errorf("Failed to get country: %v", err)
		}
		return ""
	}

	return conf.Country
}

func getConfig(timeout time.Duration) (*userconfig.UserConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return userconfig.GetConfig(ctx)
}

// OnRefresh returns a chan that will signal when the goelocation has changed.
func OnRefresh() <-chan bool {
	ch := make(chan bool, 1)
	mx.Lock()
	watchers = append(watchers, ch)
	mx.Unlock()
	return ch
}
