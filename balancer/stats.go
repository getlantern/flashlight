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
	return fmt.Sprintf("%72s: Success: %5d (%d)\tFailure: %5d / %5d (%d)\tEMA Latency: %v", d.Label, successes, consecSuccesses, failures, attempts, consecFailures, d.emaLatency.Get())
}
