package common

import (
	"os"
	"strconv"

	"github.com/getlantern/golog"
	serverCommon "github.com/getlantern/lantern-server/common"
)

var (
	// StagingMode if true, run Lantern against our staging infrastructure.
	// This is set by the linker using -ldflags
	StagingMode = "false"

	Staging = false

	ProAPIHost = "api.getiantem.org"

	AuthServerAddr  = serverCommon.AuthServerAddr
	YinbiServerAddr = serverCommon.YinbiServerAddr

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
	}
	forceAds, _ = strconv.ParseBool(os.Getenv("FORCEADS"))
}

// ForceAds indicates whether adswapping should be forced to 100%
func ForceAds() bool {
	return forceAds
}
