package desktop

// startFeaturesService starts a new features service that dispatches features to the
// frontend.
func startFeaturesService(listeningFunc func(map[string]bool), enabledFeatures func() map[string]bool,
	chans ...<-chan bool) {

	for _, ch := range chans {
		go func(c <-chan bool) {
			for range c {
				listeningFunc(enabledFeatures())
			}
		}(ch)
	}
}
