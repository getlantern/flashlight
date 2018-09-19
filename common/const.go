package common

import (
	"os"
	"strconv"
)

var (
	// StagingMode if true, run Lantern against our staging infrastructure.
	// This is set by the linker using -ldflags
	StagingMode = "false"

	Staging = false

	ProAPIHost = "api.getiantem.org"

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
	Staging, err = strconv.ParseBool(StagingMode)
	if err != nil {
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
