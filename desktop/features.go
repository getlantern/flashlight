package desktop

import (
	"github.com/getlantern/flashlight/ws"
)

// startFeaturesService starts a new features service that dispatches features to the
// frontend.
func startFeaturesService(channel ws.UIChannel, enabledFeatures func() map[string]bool,
	chans ...<-chan bool) {
	if service, err := channel.Register("features", func(write func(interface{})) {
		log.Debugf("Sending features enabled to new client")
		write(enabledFeatures())
	}); err != nil {
		log.Errorf("Unable to serve enabled features to UI: %v", err)
	} else {
		go func() {
			for _, ch := range chans {
				go func(c <-chan bool) {
					for range c {
						select {
						case service.Out <- enabledFeatures():
							// ok
						default:
							// don't block if no-one is listening
						}
					}
				}(ch)
			}
		}()
	}
}
