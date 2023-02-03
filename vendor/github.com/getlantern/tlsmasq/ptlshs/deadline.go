package ptlshs

import (
	"sync"
	"time"
)

// deadline is an abstraction for handling timeouts. This code is taken from the pipeDeadline type
// defined in https://golang.org/src/net/pipe.go.
type deadline struct {
	mu     sync.Mutex // Guards timer and cancel
	timer  *time.Timer
	cancel chan struct{} // Must be non-nil

	// These fields were added by us; they were not ported from https://golang.org/src/net/pipe.go.
	t      time.Time
	closed bool
}

func newDeadline() deadline {
	return deadline{cancel: make(chan struct{})}
}

// get the current deadline.
// This function was added by us; it was not ported from https://golang.org/src/net/pipe.go.
func (d *deadline) get() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.t
}

// set sets the point in time when the deadline will time out.
// A timeout event is signaled by closing the channel returned by waiter.
// Once a timeout has occurred, the deadline can be refreshed by specifying a
// t value in the future.
//
// A zero value for t prevents timeout.
func (d *deadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.t = t
	if d.timer != nil && !d.timer.Stop() {
		<-d.cancel // Wait for the timer callback to finish and close cancel
	}
	d.timer = nil

	// Time is zero, then there is no deadline.
	closed := isClosedChannel(d.cancel)
	if t.IsZero() {
		if closed {
			d.cancel = make(chan struct{})
		}
		return
	}

	// Time in the future, setup a timer to cancel in the future.
	if dur := time.Until(t); dur > 0 {
		if closed {
			d.cancel = make(chan struct{})
		}
		d.timer = time.AfterFunc(dur, func() {
			close(d.cancel)
		})
		return
	}

	// Time in the past, so close immediately.
	if !closed {
		close(d.cancel)
	}
}

// wait returns a channel that is closed when the deadline is exceeded.
func (d *deadline) wait() chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancel
}

// close the deadline. Note that this does not close the channel returned by wait.
// This function was added by us; it was not ported from https://golang.org/src/net/pipe.go.
func (d *deadline) close() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.closed = true
	d.mu.Unlock()
}

// This function was added by us; it was not ported from https://golang.org/src/net/pipe.go.
func (d *deadline) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}
