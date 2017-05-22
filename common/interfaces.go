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

// UserConfig is a common interface to get current user ID and token.
type UserConfig interface {
	GetUserID() int64
	GetToken() string
}

type userConfigWrapper struct {
	getUserID func() int64
	getToken  func() string
}

func (u userConfigWrapper) GetUserID() int64 {
	return u.getUserID()
}

func (u userConfigWrapper) GetToken() string {
	return u.getToken()
}

// WrapUserConfig wraps independent functions to an UserConfig interface.
func WrapUserConfig(getUserID func() int64, getToken func() string) UserConfig {
	return userConfigWrapper{getUserID, getToken}
}

// Settings provides a common way to inject the settings user can change anytime.
type Settings interface {
	// When true, flashlight resolves the IP address first and dial directly if
	// the IP belongs to user's current geolocated country.
	UseShortcut() bool
	// When true, flashlight tries dial directly first. If the attempt fails
	// or doesn't succeed in a reasonably time, dial again via proxy servers.
	UseDetour() bool
	// When true, flashlight will send logs and statistics to help diagnose.
	IsAutoReport() bool
}

type settingsWrapper struct {
	useShortcut  func() bool
	useDetour    func() bool
	isAutoReport func() bool
}

func (u settingsWrapper) UseShortcut() bool {
	return u.useShortcut()
}

func (u settingsWrapper) UseDetour() bool {
	return u.useDetour()
}

func (u settingsWrapper) IsAutoReport() bool {
	return u.isAutoReport()
}

// WrapUserConfig wraps independent functions to a Settings interface.
func WrapSettings(useDetour func() bool, useShortcut func() bool, isAutoReport func() bool) Settings {
	return settingsWrapper{useShortcut, useDetour, isAutoReport}
}

// Not transforms a function f() to return !f()
func Not(f func() bool) func() bool {
	return func() bool { return !f() }
}
