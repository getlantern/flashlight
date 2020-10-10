// +build beam

package common

const (
	// AppName is the name of this specific app.
	AppName = "Beam"

	// ProAvailable specifies whether the user can purchase pro with this version.
	ProAvailable = false

	// TrackingID is the Google Analytics tracking ID.
	TrackingID = "UA-21815217-23"

	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.ss7hc6jm.io/global.yaml.gz"

	// GlobalStagingURL is the URL for fetching the global config in a staging environment.
	GlobalStagingURL = "https://globalconfig.ss7hc6jm.io/global.yaml.gz"

	// The following are over HTTP because proxies do not forward X-Forwarded-For
	// with HTTPS and because we only support falling back to direct domain
	// fronting through the local proxy for HTTP.

	// ProxiesURL is the URL for fetching the per user proxy config.
	ProxiesURL = "http://config.ss7hc6jm.io/proxies.yaml.gz"

	// ProxiesStagingURL is the URL for fetching the per user proxy config in a staging environment.
	ProxiesStagingURL = "http://config-staging.ss7hc6jm.io/proxies.yaml.gz"
)
