package services

import (
	"time"

	mrand "math/rand/v2"

	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("flashlight.services")

type StopFn func()

// callRandomly continuously calls fn randomly between interval-jitter and interval+jitter, with
// the initial call being made immediately. fn can return a positive value to extend the wait time.
func callRandomly(
	fn func() int64,
	interval time.Duration,
	jitter time.Duration,
	done <-chan struct{},
) {
	jitterInt := jitter.Nanoseconds()
	intervalInt := interval.Nanoseconds()

	// calculate sleep time
	sleep := func(extraDelay time.Duration) <-chan time.Time {
		delay := mrand.Int64N(2*jitterInt) + intervalInt - jitterInt
		delayDuration := time.Duration(delay) + extraDelay
		logger.Debugf("Next run in %v", delayDuration)
		return time.After(delayDuration)
	}

	// make initial call to fn immediately before entering loop
	extraSeconds := fn()

	extraDelay := time.Duration(extraSeconds) * time.Second
	for {
		select {
		case <-done:
			return
		case <-sleep(extraDelay):
			start := time.Now()
			extraSeconds = fn()
			elapsed := time.Since(start)
			extraDelay = time.Duration(extraSeconds)*time.Second - elapsed
			extraDelay = max(extraDelay, 0)
		}
	}
}
