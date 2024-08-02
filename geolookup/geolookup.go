package geolookup

import (
	"context"
	"sync"
	"time"

	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/golog"

	proxyconfig "github.com/getlantern/flashlight/v7/config/proxy"
)

var (
	log = golog.LoggerFor("flashlight.geolookup")

	// _country and _ip are eventual values that hold the current country and IP as strings.
	_country = eventual.NewValue()
	_ip      = eventual.NewValue()

	watchers []chan bool
	mx       sync.Mutex
)

func init() {
	proxyconfig.OnConfigChange(func(old, new *proxyconfig.ProxyConfig) {
		oldCountry, _ := _country.Get(eventual.DontWait)
		oldIP, _ := _ip.Get(eventual.DontWait)

		_country.Set(new.Country)
		_ip.Set(new.Ip)

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

	ip, err := _ip.Get(ctx)
	if err != nil {
		log.Errorf("Failed to get IP: %w", err)
		return ""
	}

	return ip.(string)
}

// GetCountry gets the country. If the country hasn't been determined yet, waits up to the given
// timeout for country to become available.
func GetCountry(timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	country, err := _country.Get(ctx)
	if err != nil {
		log.Errorf("Failed to get country: %w", err)
		return ""
	}

	return country.(string)
}

// OnRefresh returns a chan that will signal when the goelocation has changed.
func OnRefresh() <-chan bool {
	ch := make(chan bool, 1)
	mx.Lock()
	watchers = append(watchers, ch)
	mx.Unlock()
	return ch
}
