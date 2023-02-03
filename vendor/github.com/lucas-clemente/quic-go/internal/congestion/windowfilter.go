package congestion

import (
	"time"
)

type windowFilterSample struct {
	Sample int64
	Time   time.Time
}

type windowFilter struct {
	windowLength   time.Duration         // Time length of window.
	zeroValue      int64                 // Uninitialized value of T.
	estimates      [3]windowFilterSample // Best estimate is element 0.
	MaxOrMinFilter bool                  // If true, max Filter, if False, min filter
}

func (wF *windowFilter) Update(newSample int64, newTime time.Time) {
	// Reset all estimates if they have not yet been initialized, if new sample
	// is a new best, or if the newest recorded estimate is too old.
	if wF.estimates[0].Sample == wF.zeroValue ||
		wF.compareInternal(newSample, wF.estimates[0].Sample) ||
		newTime.UnixNano()-wF.estimates[2].Time.UnixNano() > int64(wF.windowLength) {

		wF.Reset(newSample, newTime)
		return
	}

	if wF.compareInternal(newSample, wF.estimates[1].Sample) {
		wF.estimates[1] = windowFilterSample{
			Sample: newSample,
			Time:   newTime,
		}
		wF.estimates[2] = wF.estimates[1]

	} else if wF.compareInternal(newSample, wF.estimates[2].Sample) {
		wF.estimates[2] = windowFilterSample{
			Sample: newSample,
			Time:   newTime,
		}
	}

	// Expire and update estimates as necessary.
	if newTime.UnixNano()-wF.estimates[0].Time.UnixNano() > int64(wF.windowLength) {
		// The best estimate hasn't been updated for an entire window, so promote
		// second and third best estimates.
		wF.estimates[0] = wF.estimates[1]
		wF.estimates[1] = wF.estimates[2]
		wF.estimates[2] = windowFilterSample{
			Sample: newSample,
			Time:   newTime,
		}
		// Need to iterate one more time. Check if the new best estimate is
		// outside the window as well, since it may also have been recorded a
		// long time ago. Don't need to iterate once more since we cover that
		// case at the beginning of the method.
		if newTime.UnixNano()-wF.estimates[0].Time.UnixNano() > int64(wF.windowLength) {
			wF.estimates[0] = wF.estimates[1]
			wF.estimates[1] = wF.estimates[2]
		}
		return
	}
	if wF.estimates[1].Sample == wF.estimates[0].Sample &&
		time.Duration(newTime.UnixNano())-time.Duration(wF.estimates[1].Time.UnixNano()) > wF.windowLength>>2 {
		// A quarter of the window has passed without a better sample, so the
		// second-best estimate is taken from the second quarter of the window.
		wF.estimates[2] = windowFilterSample{
			Sample: newSample,
			Time:   newTime,
		}
		wF.estimates[1] = wF.estimates[2]
		return
	}

	if wF.estimates[2].Sample == wF.estimates[1].Sample &&
		time.Duration(newTime.UnixNano())-time.Duration(wF.estimates[2].Time.UnixNano()) > wF.windowLength>>1 {
		// We've passed a half of the window without a better estimate, so take
		// a third-best estimate from the second half of the window.
		wF.estimates[2] = windowFilterSample{
			Sample: newSample,
			Time:   newTime,
		}
	}
}

// Resets all estimates to new sample.
func (wF *windowFilter) Reset(newSample int64, newTime time.Time) {
	nS := windowFilterSample{
		Sample: newSample,
		Time:   newTime,
	}
	wF.estimates[0] = nS
	wF.estimates[1] = nS
	wF.estimates[2] = nS
}

func (wF *windowFilter) compareInternal(a, b int64) bool {
	if wF.MaxOrMinFilter {
		return a > b
	}
	return a < b
}

func (wF *windowFilter) GetBest() int64 {
	return wF.estimates[0].Sample
}

func (wF *windowFilter) GetSecondBest() int64 {
	return wF.estimates[1].Sample
}

func (wF *windowFilter) GetThirdBest() int64 {
	return wF.estimates[2].Sample
}
