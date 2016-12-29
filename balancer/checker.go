package balancer

import (
	"math/rand"
	"time"

	"github.com/sparrc/go-ping"
)

const (
	pingInterval     = 10 * time.Second
	minCheckInterval = 10 * time.Second
	maxCheckInterval = 1 * time.Hour
)

var (
	// Whom to ping to check network reachability
	pingTarget = "github.com"

	// Hardcoded list of URLs to check latency
	checkTargets = []string{
		"https://www.google.com/favicon.ico",
		"https://www.facebook.com/humans.txt",
		"https://www.tumblr.com/humans.txt",
		"https://www.youtube.com/robots.txt",
	}
)

type checker struct {
	d             *dialer
	checkInterval time.Duration
	stopCh        chan bool
}

func newChecker(d *dialer, opts *Opts) *checker {
	return &checker{
		d:      d,
		stopCh: make(chan bool, 1),
	}
}

func (c *checker) runChecks() {
	checkInterval := c.minCheckInterval
	pingTimer := time.newTimer(0)  // ping immediately on start
	checkTimer := time.NewTimer(0) // check immediately on Start

	check := func() {
		successful := c.check()
		if successful {
			// Exponentially back off checkInterval capped to maxCheckInterval
			checkInterval *= 2
			if checkInterval > c.maxCheckInterval {
				checkInterval = c.maxCheckInterval
			}
		}
		nextCheck := randomize(checkInterval)
		checkTimer.Reset(nextCheck)
	}

	for {
		select {
		case <-c.resetCh:
			log.Debugf("Recheck forced for %v", c.d.Label)
			checkInterval = c.minCheckInterval
			latency := c.d.emaLatency.Get()
			if latency > checkInterval {
				// Don't check more frequently than it takes to actually run the check
				checkInterval = latency
			}
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

func (c *checker) check() bool {
	log.Debugf("Checking dialer: %v", c.d.Label)
	successful := c.doCheck()
	if successful {
		log.Debugf("Succeeded Checking dialer: %v", c.d.Label)
		// If the check succeeded, we mark down a success.
		c.d.markSuccess()
	} else {
		log.Debugf("Failed Checking dialer: %v", c.d.Label)
		c.d.markFailure()
	}
	return successful
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
				// to within 2x of existing latency.
				latency = cap
			} else if latency < oldLatency/2 {
				// On major reduction in latency, force a recheck to see if this is a
				// more permanent change.
				c.forceRecheck()
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
