package balancer

import (
	"sync/atomic"
	"time"
)

const (
	// floating point values are stored to this scale (3 digits behind decimal
	// point).
	scale = 1000
)

// ema holds the Exponential Moving Average of a float64 with a the given
// default α value and a fixed scale of 3 digits. Safe to access concurrently.
// https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average.
type ema struct {
	defaultAlpha float64
	v            int64
}

// newEMA creates an ema with initial value and alpha
func newEMA(initial float64, defaultAlpha float64) *ema {
	return &ema{defaultAlpha: defaultAlpha, v: scaleToInt(initial)}
}

// Like newEMA but using time.Duration
func newEMADuration(initial time.Duration, alpha float64) *ema {
	return newEMA(float64(initial), alpha)
}

// updateAlpha calculates and stores new EMA based on the duration and α
// value passed in.
func (e *ema) updateAlpha(v float64, α float64) float64 {
	oldEMA := scaleFromInt(atomic.LoadInt64(&e.v))
	newEMA := (1-α)*oldEMA + α*v
	atomic.StoreInt64(&e.v, scaleToInt(newEMA))
	return newEMA
}

// like updateAlpha but using the default alpha
func (e *ema) update(v float64) float64 {
	return e.updateAlpha(v, e.defaultAlpha)
}

// Like update but using time.Duration
func (e *ema) updateDuration(v time.Duration) time.Duration {
	return time.Duration(e.update(float64(v)))
}

// Set sets the EMA directly.
func (e *ema) set(v float64) {
	atomic.StoreInt64(&e.v, scaleToInt(v))
}

// Like set but using time.Duration
func (e *ema) setDuration(v time.Duration) {
	e.set(float64(v))
}

// Get gets the EMA
func (e *ema) get() float64 {
	return scaleFromInt(atomic.LoadInt64(&e.v))
}

// Like get but using time.Duration
func (e *ema) getDuration() time.Duration {
	return time.Duration(e.get())
}

func scaleToInt(f float64) int64 {
	return int64(f * scale)
}

func scaleFromInt(i int64) float64 {
	return float64(i) / scale
}
