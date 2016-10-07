package balancer

import (
	"sync/atomic"
	"time"
)

// emaDuration holds the Exponential Moving Average of time.Duration with an α
// of 1/ia. Use multiplicative inverse of α simply for integer arithmetic.
// Safe to access concurrently.
// https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average.
type emaDuration struct {
	ia int64 // the inverse of α in the algorithm
	d  int64
}

// newEMADuration creates an emaDuration with initial value and alpha
func newEMADuration(initial time.Duration, alpha float32) *emaDuration {
	return &emaDuration{d: int64(initial), ia: int64(1 / alpha)}
}

// UpdateWith calculates and stores new EMA based on the duration passed in.
// The new EMA is returned.
func (e *emaDuration) UpdateWith(d time.Duration) time.Duration {
	oldEMA := atomic.LoadInt64(&e.d)
	// same as ((1 - α) * oldEMA + α * d)
	newEMA := ((e.ia-1)*oldEMA + int64(d)) / e.ia
	atomic.StoreInt64(&e.d, newEMA)
	return time.Duration(newEMA)
}

// Set sets the EMA directly.
func (e *emaDuration) Set(d time.Duration) {
	atomic.StoreInt64(&e.d, int64(d))
}

// Get gets the EMA as duration
func (e *emaDuration) Get() time.Duration {
	return time.Duration(e.GetInt64())
}

// GetInt64 gets the EMA as int64
func (e *emaDuration) GetInt64() int64 {
	return atomic.LoadInt64(&e.d)
}
