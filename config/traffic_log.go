package config

import "time"

// TrafficLogConfig is configuration for github.com/getlantern/trafficlog-flashlight.
type TrafficLogConfig struct {
	// TODO: think about existing clients pulling down old config values in Global

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

	// The minimum amount of time to wait before re-prompt the user since the last time the user
	// denied permission to install the traffic log. If 0, the user will never be re-prompted.
	WaitTimeSinceDenial time.Duration
}
