package services

import (
	"time"

	mrand "math/rand"

	"github.com/getlantern/golog"
)

// callRandomly continuously calls fn on random intervals between minWaitSeconds and maxWaitSeconds,
// until ctx is done, with the initial call being made immediately. fn can return a positive value
// to extend the wait time.
func callRandomly(
	fn func() int64,
	minWaitSeconds int64,
	maxWaitSeconds int64,
	done <-chan struct{},
	logger golog.Logger,
) {
	// calculate sleep time
	sleep := func(extraSeconds int64, elapsed time.Duration) <-chan time.Time {
		delay := mrand.Int63n(maxWaitSeconds) + minWaitSeconds + extraSeconds
		delayDuration := time.Duration(delay)*time.Second + elapsed
		logger.Debugf("Next run in %v", delayDuration)
		return time.After(delayDuration)
	}

	var (
		extraDelay int64
		elapsed    time.Duration
	)

	// make initial call to fn immediately before entering loop
	extraDelay = fn()

	for {
		select {
		case <-done:
			return
		case <-sleep(extraDelay, elapsed):
			start := time.Now()
			extraDelay = fn()
			elapsed = time.Since(start)
		}
	}
}
