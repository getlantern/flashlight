// +build !lantern
// +build !beam

package common

const (
	// AppName is the name of this particular app, such as Lantern or Beam
	AppName = "Default"

	// ProAvailable specifies whether the user can purchase pro with this version.
	ProAvailable = false

	// TrackingID is the Google Analytics tracking ID.
	TrackingID = "default"

	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	// GlobalStagingURL is the URL for fetching the global config in a staging environment.
	GlobalStagingURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	// The following are over HTTP because proxies do not forward X-Forwarded-For
	// with HTTPS and because we only support falling back to direct domain
	// fronting through the local proxy for HTTP.

	// ProxiesURL is the URL for fetching the per user proxy config.
	ProxiesURL = "http://config.getiantem.org/proxies.yaml.gz"

	// ProxiesStagingURL is the URL for fetching the per user proxy config in a staging environment.
	ProxiesStagingURL = "http://config-staging.getiantem.org/proxies.yaml.gz"
)
