// Package services provides the background services of the flashlight application. These services
// are responsible for fetching and updating the proxy configuration, and for reporting to the
// bypass server to detect if proxies are blocked. Requests to the servers are made at random
// intervals to prevent the thundering herd problem and help avoid detection.
package services

import (
	"time"

	mrand "math/rand/v2"

	"github.com/getlantern/golog"
)

const jitter = 2 * time.Minute

var logger = golog.LoggerFor("flashlight.services")

type StopFn func()

// callRandomly continuously calls fn randomly between interval-jitter and interval+jitter, with
// the initial call being made immediately. fn can return a positive value to extend the wait time.
func callRandomly(fn func() int64, interval time.Duration, done <-chan struct{}) {
	callRandomlyWithJitter(fn, interval, jitter, done)
}

func callRandomlyWithJitter(
	fn func() int64,
	interval time.Duration,
	jitter time.Duration,
	done <-chan struct{},
) {
	jitterInt := jitter.Milliseconds()
	intervalInt := interval.Milliseconds()

	// calculate sleep time
	sleep := func(extraDelay time.Duration) <-chan time.Time {
		delay := intervalInt
		if jitterInt > 0 {
			delay += mrand.Int64N(2*jitterInt) - jitterInt
		}

		delayDuration := time.Duration(delay)*time.Millisecond + extraDelay
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
