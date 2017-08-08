package common

import (
	"os"
	"strconv"

	"github.com/getlantern/golog"
)

var (
	// StagingMode if true, run Lantern against our staging infrastructure.
	// This is set by the linker using -ldflags
	StagingMode = "false"

	Staging = false

	ProAPIHost    = "api.getiantem.org"
	ProAPIDDFHost = "d2n32kma9hyo9f.cloudfront.net"

	log = golog.LoggerFor("flashlight.common")

	forceAds bool
)

func init() {
	initInternal()
}

// Init is a public init function that's useful in tests.
func Init() {
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
		ProAPIDDFHost = "d16igwq64x5e11.cloudfront.net"
	}
	forceAds, _ = strconv.ParseBool(os.Getenv("FORCEADS"))
}

// ForceAds indicates whether adswapping should be forced to 100%
func ForceAds() bool {
	return forceAds
}
