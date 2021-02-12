package common

import "strconv"

var (
	// ForceEnableTrafficlog is true when the traffic log should be enabled regardless of other
	// config. This can be set at build-time using -ldflags. For example,
	//   go build -ldflags "-X github.com/getlantern/flashlight/common.ForceEnableTrafficlog=true"
	ForceEnableTrafficlog = "false"

	// ForceEnableTrafficlogFeature is true when the traffic log should be enabled regardless of
	// other config. This can be set at build-time; see ForceEnableTrafficlog
	ForceEnableTrafficlogFeature = false
)

func init() {
	ForceEnableTrafficlogFeature, _ = strconv.ParseBool(ForceEnableTrafficlog)
}
