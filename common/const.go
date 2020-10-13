package common

import (
	"os"
	"strconv"

	"github.com/getlantern/golog"
)

const (
	// The following are over HTTP because proxies do not forward X-Forwarded-For
	// with HTTPS and because we only support falling back to direct domain
	// fronting through the local proxy for HTTP.

	// ProxiesURL is the URL for fetching the per user proxy config.
	ProxiesURL = "http://config.getiantem.org/proxies.yaml.gz"

	// ProxiesStagingURL is the URL for fetching the per user proxy config in a staging environment.
	ProxiesStagingURL = "http://config-staging.getiantem.org/proxies.yaml.gz"

	// UpdateServerURL is the URL of the update server.
	UpdateServerURL = "https://update.getlantern.org"
)

var (
	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	// GlobalStagingURL is the URL for fetching the global config in a staging environment.
	GlobalStagingURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	// StagingMode if true, run Lantern against our staging infrastructure.
	// This is set by the linker using -ldflags
	StagingMode = "false"

	Staging = false

	ProAPIHost = "api.getiantem.org"

	ReplicaSearchAPIHost = "replica-search.lantern.io"

	log = golog.LoggerFor("flashlight.common")

	forceAds bool
)

func init() {
	initInternal()
}

// ForceStaging forces staging mode.
func ForceStaging() {
	StagingMode = "true"
	initInternal()
}

func initInternal() {
	var err error
	log.Debugf("****************************** stagingMode: %v", StagingMode)
	Staging, err = strconv.ParseBool(StagingMode)
	if err != nil {
		log.Errorf("Error parsing boolean flag: %v", err)
		return
	}
	if Staging {
		ProAPIHost = "api-staging.getiantem.org"
		ReplicaSearchAPIHost = "replica-search-staging.lantern.io"
	}
	forceAds, _ = strconv.ParseBool(os.Getenv("FORCEADS"))
}

// ForceAds indicates whether adswapping should be forced to 100%
func ForceAds() bool {
	return forceAds
}
