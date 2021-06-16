package config

import (
	"os"
	"strconv"
	"time"
)

var (
	// EnableReplicaFeatures enables replica related features via the REPLICA build time variable.
	EnableReplicaFeatures = "false"

	// EnableReplica is true when we should force replica to be enabled regardless of other configuration.
	EnableReplica = false

	// EnableYinbiFeatures enables yinbi wallet related features via the YINBI build time variable
	EnableYinbiFeatures = "false"

	// EnableYinbi is true when we shuld force replica to be enabled regardless of other configuration.
	EnableYinbi = false

	// EnableTrafficlogFeatures is true when the traffic log should be enabled regardless of other
	// config. This can be set at build-time using -ldflags. For example,
	//   go build -ldflags "-X github.com/getlantern/flashlight/common.EnableTrafficlogFeatures=true"
	// More commonly you'd enable this via the TRAFFICLOG build time variable.
	EnableTrafficlogFeatures = "false"

	// EnableTrafficlog is true when the traffic log should be enabled regardless of
	// other config. This can be set at build-time; see EnableTrafficlogFeatures
	EnableTrafficlog = false

	// ForcedTrafficLogOptions contains the traffic log options that should be used if the
	// traffic log is force enabled at build time.
	ForcedTrafficLogOptions = &TrafficLogOptions{
		CaptureBytes:               10 * 1024 * 1024,
		SaveBytes:                  10 * 1024 * 1024,
		CaptureSaveDuration:        5 * time.Minute,
		Reinstall:                  true,
		WaitTimeSinceFailedInstall: 24 * time.Hour,
		UserDenialThreshold:        3,
		TimeBeforeDenialReset:      24 * time.Hour,
		FailuresThreshold:          3,
		TimeBeforeFailureReset:     24 * time.Hour,
	}
)

func init() {
	enableFeature := func(featureEnabled, envVarName string) bool {
		enabled := func(val string) bool {
			if enable, err := strconv.ParseBool(val); err == nil {
				return enable
			}
			return false
		}
		envVal := os.Getenv(envVarName)
		if envVal != "" {
			return enabled(envVal)
		}
		return enabled(featureEnabled)
	}

	EnableTrafficlog = enableFeature(EnableTrafficlogFeatures, "TRAFFICLOG")
	EnableReplica = enableFeature(EnableReplicaFeatures, "REPLICA")
	EnableYinbi = enableFeature(EnableYinbiFeatures, "YINBI")
}
