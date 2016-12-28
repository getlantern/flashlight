package balancer

import (
	"math/rand"
	"time"
)

const (
	defaultMinCheckInterval = 10 * time.Second
	defaultMaxCheckInterval = 1 * time.Hour
)

var (
	// Hardcoded list of URLs to check latency
	checkTargets = []string{
		"https://www.google.com/favicon.ico",
		"https://www.facebook.com/humans.txt",
		"https://www.tumblr.com/humans.txt",
		"https://www.youtube.com/robots.txt",
	}
)

type checker struct {
	d                *dialer
	checkInterval    time.Duration
	minCheckInterval time.Duration
	maxCheckInterval time.Duration
	resetCh          chan bool
	stopCh           chan bool
}

func newChecker(d *dialer, opts *Opts) *checker {
	c := &checker{
		d:                d,
		minCheckInterval: opts.MinCheckInterval,
		maxCheckInterval: opts.MaxCheckInterval,
		resetCh:          make(chan bool, 1),
		stopCh:           make(chan bool, 1),
	}

	if c.minCheckInterval <= 0 {
		c.minCheckInterval = defaultMinCheckInterval
	}
	if c.maxCheckInterval <= 0 {
		c.maxCheckInterval = defaultMaxCheckInterval
	}

	return c
}

func (c *checker) runChecks() {
	checkInterval := c.minCheckInterval
	checkTimer := time.NewTimer(0) // check immediately on Start

	check := func() {
		checkInterval = c.minCheckInterval
		c.check()
		// Exponentially back off checkInterval capped to MaxCheckInterval
		checkInterval *= 2
		if checkInterval > c.maxCheckInterval {
			checkInterval = c.maxCheckInterval
		}
		nextCheck := randomize(checkInterval)
		checkTimer.Reset(nextCheck)
	}

	for {
		select {
		case <-c.resetCh:
			check()
		case <-checkTimer.C:
			check()
		case <-c.stopCh:
			log.Trace("Checker stopped")
			checkTimer.Stop()
			return
		}
	}
}

func (c *checker) stop() {
	select {
	case c.stopCh <- true:
		// stopping
	default:
		// stop already requested
	}
}

func (c *checker) check() {
	log.Debugf("Checking dialer: %v", c.d.Label)
	if c.doCheck() {
		log.Debugf("Succeeded Checking dialer: %v", c.d.Label)
		// If the check succeeded, we mark down a success.
		c.d.markSuccess()
	} else {
		log.Debugf("Failed Checking dialer: %v", c.d.Label)
		c.d.markFailure()
	}
}

func (c *checker) doCheck() bool {
	ok, latency := c.d.Check(checkTargets)
	if ok {
		c.d.markSuccess()
		oldLatency := c.d.emaLatency.Get()
		if oldLatency > 0 {
			cap := oldLatency * 2
			if latency > cap {
				// To avoid random large fluctuations in latency, keep change in latency
				// to within 2x of existing latency. If this happens, force a recheck.
				latency = cap
			}
		}
		newEMA := c.d.emaLatency.UpdateWith(latency)
		log.Tracef("Updated dialer %s emaLatency to %v", c.d.Label, newEMA)
	} else {
		log.Tracef("Dialer %s failed check", c.d.Label)
	}

	return ok
}

// adds randomization to make requests less distinguishable on the network.
func randomize(d time.Duration) time.Duration {
	return time.Duration((d.Nanoseconds() / 2) + rand.Int63n(d.Nanoseconds()))
}
