package common

// StatsTracker is a common interface to receive user perceptible Lantern stats
type StatsTracker interface {
	// SetActiveProxyLocation updates the location of last successfully dialed
	// proxy. countryCode is in ISO Alpha-2 form, see
	// http://www.nationsonline.org/oneworld/country_code_list.htm
	SetActiveProxyLocation(city, country, countryCode string)
	// IncHTTPSUpgrades indicates that Lantern client redirects a HTTP request
	// to HTTPS via HTTPSEverywhere.
	IncHTTPSUpgrades()
	// IncAdsBlocked indicates that a proxy request is blocked per easylist rules.
	IncAdsBlocked()
}
