package common

import (
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/getlantern/golog"
)

const (
	// The following are over HTTP because proxies do not forward X-Forwarded-For
	// with HTTPS and because we only support falling back to direct domain
	// fronting through the local proxy for HTTP.

	// Lantern environment types
	DevEnvironment = "development"

	// ProxiesURL is the URL for fetching the per user proxy config.
	ProxiesURL = "http://config.getiantem.org/proxies.yaml.gz"

	// ProxiesStagingURL is the URL for fetching the per user proxy config in a staging environment.
	ProxiesStagingURL = "http://config-staging.getiantem.org/proxies.yaml.gz"

	// Sentry Configurations
	SentryTimeout         = time.Second * 30
	SentryMaxMessageChars = 8000

	// UpdateServerURL is the URL of the update server. Different applications
	// hit the server on separate paths "/update/<AppName>".
	UpdateServerURL = "https://update.getlantern.org/"
)

var (
	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	// GlobalStagingURL is the URL for fetching the global config in a staging environment.
	GlobalStagingURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	// StagingMode if true, run Lantern against our staging infrastructure.
	// This is set by the linker using -ldflags
	StagingMode = "false"

	environment string

	Staging = false

	AuthAPIHost        = "https://auth4.lantern.network"
	AuthStagingAPIHost = "https://auth-staging.lantern.network"

	ProAPIHost        = "api.getiantem.org"
	ProStagingAPIHost = "api-staging.getiantem.org"

	ReplicaSearchAPIHost        = "replica-search.lantern.io"
	ReplicaSearchStagingAPIHost = "replica-search-staging.lantern.io"

	ReplicaServiceEndpoint = url.URL{Scheme: "https", Host: ReplicaSearchAPIHost}

	log = golog.LoggerFor("flashlight.common")

	forceAds bool
)

func init() {
	environment = os.Getenv("ENVIRONMENT")
	initInternal()
}

// ForceStaging forces staging mode.
func ForceStaging() {
	StagingMode = "true"
	initInternal()
}

func isDevEnvironment() bool {
	return environment == DevEnvironment
}

func initInternal() {
	var err error
	log.Debugf("****************************** stagingMode: %v", StagingMode)
	Staging, err = strconv.ParseBool(StagingMode)
	if err != nil {
		log.Errorf("Error parsing boolean flag: %v", err)
		return
	}
	if Staging || isDevEnvironment() {
		AuthServerAddr = AuthStagingAPIHost
		ReplicaSearchAPIHost = ReplicaSearchStagingAPIHost
		useYinbiStaging()
	} else if Staging {
		ProAPIHost = ProStagingAPIHost
	}
	forceAds, _ = strconv.ParseBool(os.Getenv("FORCEADS"))
}

// ForceAds indicates whether adswapping should be forced to 100%
func ForceAds() bool {
	return forceAds
}
