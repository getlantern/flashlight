package app

type statsSink struct{}

func (m statsSink) SetActiveProxyLocation(city, country, countryCode string) {}
func (m statsSink) IncHTTPSUpgrades()                                        {}
func (m statsSink) IncAdsBlocked()                                           {}
