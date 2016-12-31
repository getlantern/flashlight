package balancer

import (
	"fmt"
	"sync/atomic"
)

// stats are basic dialing stats
type stats struct {
	attempts  int64
	successes int64
	failures  int64
}

func (s *stats) String(d *dialer) string {
	successes := atomic.LoadInt64(&s.successes)
	consecSuccesses := d.ConsecSuccesses()
	failures := atomic.LoadInt64(&s.failures)
	consecFailures := d.ConsecFailures()
	attempts := atomic.LoadInt64(&s.attempts)
	return fmt.Sprintf("Dialer Stats for %v\t\tSuccess: %7d / %7d (%7d)\t\tFailure: %7d / %7d (%7d)\t\tEMA Latency: %v", d.Label, successes, attempts, consecSuccesses, failures, attempts, consecFailures, d.emaLatency.Get())
}
