package balancer

import (
	"fmt"
	"sync/atomic"
)

// stats are basic dialing stats
type stats struct {
	attempts     int64
	successes    int64
	failures     int64
	estBandwidth int64 // MBps scaled by 1000
}

func (s *stats) String(d *dialer) string {
	successes := atomic.LoadInt64(&s.successes)
	consecSuccesses := d.ConsecSuccesses()
	failures := atomic.LoadInt64(&s.failures)
	consecFailures := d.ConsecFailures()
	attempts := atomic.LoadInt64(&s.attempts)
	return fmt.Sprintf("%s  S: %4d / %4d (%d)\tF: %4d / %4d (%d)\tL: %5.0fms\tBW: %3.2fMbps", d.Label, successes, attempts, consecSuccesses, failures, attempts, consecFailures, d.EstLatency().Seconds()*1000, d.EstBandwidth())
}
