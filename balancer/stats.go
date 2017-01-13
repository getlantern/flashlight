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
	return fmt.Sprintf("%s  S: %4d / %4d (%d)\tF: %4d / %4d (%d)\tRTT: %v\tPLR: %1.2f%%\tT: %3.2fmbps", d.Label, successes, attempts, consecSuccesses, failures, attempts, consecFailures, d.emaRTT.GetDuration(), d.emaPLR.Get(), float64(atomic.LoadInt64(&d.estimatedThroughput))/1000000)
}
