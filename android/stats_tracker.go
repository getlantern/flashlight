package android

type statsTracker struct{}

func (m statsTracker) SetActiveProxyLocation(city, country, countryCode string) {}
func (m statsTracker) IncHTTPSUpgrades()                                        {}
func (m statsTracker) IncAdsBlocked()                                           {}
