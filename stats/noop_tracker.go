package stats

var NoopTracker = &noopTracker{}

// noopTracker is an implementation of Tracker that does nothing
type noopTracker struct{}

func (t *noopTracker) Latest() Stats {
	return Stats{}
}

func (t *noopTracker) AddListener(func(newStats Stats)) (close func()) {
	return func() {}
}

func (t *noopTracker) SetActiveProxyLocation(city, country, countryCode string) {}

func (t *noopTracker) IncHTTPSUpgrades() {}

func (t *noopTracker) IncAdsBlocked() {}

func (t *noopTracker) SetDisconnected(val bool) {}

func (t *noopTracker) SetHasSucceedingProxy(val bool) {}

func (t *noopTracker) SetHitDataCap(val bool) {}

func (t *noopTracker) SetIsPro(val bool) {}

func (t *noopTracker) SetYinbiEnabled(val bool) {}

func (t *noopTracker) SetAlert(alertType AlertType, details string, transient bool) {}

func (t *noopTracker) ClearAlert(alertType AlertType) {}
