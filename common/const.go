package common

import (
	"os"
	"strconv"
	"time"

	"github.com/getlantern/golog"
)

const (
	// The following are over HTTP because proxies do not forward X-Forwarded-For
	// with HTTPS and because we only support falling back to direct domain
	// fronting through the local proxy for HTTP.

	// ProxiesURL is the URL for fetching the per user proxy config.
	ProxiesURL = "http://config.getiantem.org/proxies.yaml.gz"

	// Sentry Configurations
	SentryTimeout         = time.Second * 30
	SentryMaxMessageChars = 8000

	// UpdateServerURL is the URL of the update server. Different applications
	// hit the server on separate paths "/update/<AppName>".
	UpdateServerURL = "https://update.getlantern.org/"
)

var (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
)

var (
	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	ProAPIHost = "api.getiantem.org"

	log = golog.LoggerFor("flashlight.common")

	forceAds bool

	// Set by the linker using -ldflags in the project's Makefile.
	// Defaults to 'production' so as not to mistakingly push development work
	// to a production environment
	Environment = "production"
)

// ForceAds indicates whether adswapping should be forced to 100%
func ForceAds() bool {
	forceAds, _ = strconv.ParseBool(os.Getenv("FORCEADS"))
	return forceAds
}
