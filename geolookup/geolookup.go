package geolookup

import (
	"context"
	"sync"
	"time"

	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/golog"

	userconfig "github.com/getlantern/flashlight/v7/config/user"
)

var (
	log = golog.LoggerFor("flashlight.geolookup")

	// country and ip are eventual values that hold the current country and IP as strings.
	country = eventual.NewValue()
	ip      = eventual.NewValue()

	watchers []chan bool
	mx       sync.Mutex
)

func init() {
	userconfig.OnConfigChange(func(old, new *userconfig.UserConfig) {
		oldCountry, _ := country.Get(eventual.DontWait)
		oldIP, _ := ip.Get(eventual.DontWait)

		country.Set(new.Country)
		ip.Set(new.Ip)

		// if the country or IP has changed, notify watchers
		if oldCountry != new.Country || oldIP != new.Ip {
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	i, err := ip.Get(ctx)
	if err != nil {
		log.Errorf("Failed to get IP: %v", err)
		return ""
	}

	return i.(string)
}

// GetCountry gets the country. If the country hasn't been determined yet, waits up to the given
// timeout for country to become available.
func GetCountry(timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c, err := country.Get(ctx)
	if err != nil {
		log.Errorf("Failed to get country: %v", err)
		return ""
	}

	return c.(string)
}

// OnRefresh returns a chan that will signal when the goelocation has changed.
func OnRefresh() <-chan bool {
	ch := make(chan bool, 1)
	mx.Lock()
	watchers = append(watchers, ch)
	mx.Unlock()
	return ch
}
