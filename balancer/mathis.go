package balancer

import (
	"math"
	"time"
)

const (
	// assume maximum segment size of 1460 for Mathis throughput calculation
	mss = 1460

	nanosPerMilli = 1000000
)

// mathisThroughput estimates throughput using the Mathis equation
// See https://www.switch.ch/network/tools/tcp_throughput/?do+new+calculation=do+new+calculation
// for example.
// plr is assumed to be a percentage
// Returns value in bps
func mathisThroughput(rtt time.Duration, plr float64) int64 {
	plr = plr / 100
	if plr == 0 {
		// Assume small but measurable packet loss
		// I came up with this number by comparing the result for
		// download.thinkbroadband.com to actual download speeds.
		plr = 0.000005
	}
	return int64(8 * 1000 * 1000 * (mss / rtt.Seconds()) * (1.0 / math.Sqrt(plr)) / nanosPerMilli)
}
