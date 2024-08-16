package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallRandomlyInitialCall(t *testing.T) {
	// we use a long interval to make sure that fn is called before it goes to sleep
	interval := time.Hour

	call := make(chan struct{})
	fn := func() int64 {
		call <- struct{}{}
		return 0
	}

	started := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		started <- struct{}{}
		callRandomlyWithJitter(fn, interval, 0, done)
	}()

	// wait for the goroutine to start
	<-started
	select {
	case <-call:
	case <-time.After(200 * time.Millisecond):
		assert.Fail(t, "timed out waiting for initial call")
	}

	close(done)

}

func TestCallRandomlyJitter(t *testing.T) {
	interval := 700 * time.Millisecond
	jitter := 500 * time.Millisecond // needs to be large enough to help account for scheduling

	call := make(chan time.Time)
	fn := func() int64 {
		call <- time.Now()
		return 0
	}

	done := make(chan struct{})
	defer close(done)

	go callRandomlyWithJitter(fn, interval, jitter, done)

	// add some extra time to account for scheduling
	timeout := interval + jitter + 100*time.Millisecond

	var lastCall time.Time
	select {
	case lastCall = <-call:
	case <-time.After(timeout):
		assert.FailNow(t, "timed out waiting for first call")
	}

	// we need to check a few calls to make sure that jitter is respected
	minJitter := jitter * 2
	maxJitter := time.Duration(-minJitter)
	for i := 0; i < 4; i++ {
		select {
		case nextCall := <-call:
			nextJitter := nextCall.Sub(lastCall) - interval
			maxJitter = max(maxJitter, nextJitter)
			minJitter = min(minJitter, nextJitter)

			require.WithinDuration(t, lastCall.Add(interval), nextCall, jitter)
			lastCall = nextCall
		case <-time.After(timeout):
			assert.FailNow(t, "timed out waiting for next call")
		}
	}

	// we need to check that the jitter is actually different and not just scheduling noise
	assert.GreaterOrEqual(t, maxJitter-minJitter, 40*time.Millisecond)
}

func TestCallRandomlyExtraDelay(t *testing.T) {
	interval := 100 * time.Millisecond
	delay := time.Second // extra delay is expected in seconds

	call := make(chan time.Time)
	fn := func() int64 {
		call <- time.Now()
		return int64(delay.Seconds())
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		callRandomlyWithJitter(fn, interval, 0, done)
	}()

	var lastCall time.Time
	select {
	case lastCall = <-call:
	case <-time.After(time.Second):
		assert.FailNow(t, "timed out waiting for first call")
	}

	select {
	case nextCall := <-call:
		nextDelay := nextCall.Sub(lastCall) - interval
		// we can safely truncate to seconds because the delay is in seconds. This helps account for
		// scheduling
		assert.Equalf(
			t, delay, nextDelay.Truncate(time.Second),
			"expected next call to be %v later", delay,
		)
	case <-time.After(interval + delay + 100*time.Millisecond):
		assert.Fail(t, "timed out waiting for next call")
	}
}
