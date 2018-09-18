package proxiedsites

import (
	"sync"

	"github.com/getlantern/detour"
	"github.com/getlantern/zaplog"
	"github.com/getlantern/proxiedsites"
)

var (
	log = zaplog.LoggerFor("flashlight.proxiedsites")

	PACURL     string
	startMutex sync.Mutex
)

func Configure(cfg *proxiedsites.Config) {
	log.Info("Configuring")

	delta := proxiedsites.Configure(cfg)
	startMutex.Lock()

	if delta != nil {
		updateDetour(delta)
	}
	startMutex.Unlock()
}

func updateDetour(delta *proxiedsites.Delta) {
	log.Infof("Updating detour with %d additions and %d deletions", len(delta.Additions), len(delta.Deletions))

	// TODO: subscribe changes of geolookup and set country accordingly
	// safe to hardcode here as IR has all detection rules
	detour.SetCountry("IR")

	// for simplicity, detour matches whitelist using host:port string
	// so we add ports to each proxiedsites
	for _, v := range delta.Deletions {
		detour.RemoveFromWl(v)
	}
	for _, v := range delta.Additions {
		detour.AddToWl(v, true)
	}
}
