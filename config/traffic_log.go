package config

import "time"

// TrafficLogConfig is configuration for github.com/getlantern/trafficlog-flashlight.
type TrafficLogConfig struct {
	// If the client's platform appears in this list, traffic logging may be enabled.
	Platforms []string

	// The percent chance a client will enable traffic logging (subject to platform restrictions).
	PercentClients float64

	// Size of the traffic log's packet buffers (if enabled).
	CaptureBytes int
	SaveBytes    int

	// Whether to overwrite the traffic log binary. This may result in users being re-prompted for
	// their passwords. The binary will never be overwritten if the existing binary matches the
	// embedded version.
	Reinstall bool

	// The minimum amount of time to wait before re-prompting the user since the last time we failed
	// to install the traffic log. The most likely reason for a failed install is denial of
	// permission by the user. A value of 0 means we never re-attempt installation.
	WaitTimeSinceFailedInstall time.Duration
}
