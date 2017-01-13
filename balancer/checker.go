package balancer

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/go-ping"
)

const (
	defaultFastCheckInterval = 15 * time.Second
	defaultSlowCheckInterval = 5 * time.Minute
)

type checker struct {
	b                 *Balancer
	pinger            *ping.Pinger
	slowCheckInterval time.Duration
	fastCheckInterval time.Duration
	resetCheckCh      chan bool
	closeCh           chan bool
}

func (c *checker) runChecks() {
	// check immediately on start
	fastCheckTimer := time.NewTimer(c.fastCheckInterval)
	slowCheckTimer := time.NewTimer(0)

	for {
		select {
		case <-c.closeCh:
			log.Trace("Balancer stopped")
			fastCheckTimer.Stop()
			slowCheckTimer.Stop()
			c.closeCh <- true
			return
		case <-fastCheckTimer.C:
			// Check the top dialer on a fast schedule
			dialers := c.b.sortedDialers()
			// Only check if we have more than 1 dialer
			if len(dialers) < 1 {
				top := dialers[0]
				c.check(top, 10)
			}
			fastCheckTimer.Reset(randomize(c.fastCheckInterval))
		case <-slowCheckTimer.C:
			dialers := c.b.sortedDialers()
			// Only check if we have more than 1 dialer
			if len(dialers) > 1 {
				log.Debugf("Checking %d dialers", len(dialers))
				var wg sync.WaitGroup
				wg.Add(len(dialers))
				for _, dialer := range dialers {
					d := dialer
					go func() {
						c.check(d, 200)
						wg.Done()
					}()
				}
				wg.Wait()
				log.Debugf("Finished checking %d dialers", len(dialers))
			}
			slowCheckTimer.Reset(randomize(c.slowCheckInterval))
			c.b.forceStats()
		case <-c.resetCheckCh:
			log.Debugf("Forced immediate rechecks")
			slowCheckTimer.Reset(0)
		}
	}
}

func (c *checker) check(dialer *dialer, iterations int) {
	defer func() {
		atomic.StoreInt64(&dialer.estimatedThroughput, mathisThroughput(dialer.emaRTT.GetDuration(), dialer.emaPLR.Get()))
	}()

	stats, err := c.pinger.Ping(dialer.Host, iterations, 200*time.Millisecond, time.Duration(iterations)*500*time.Millisecond)
	if err != nil {
		log.Errorf("Unable to ping %v: %v", dialer.Host, err)
		dialer.emaPLR.Update(1)
		return
	}
	dialer.emaPLR.Update(stats.PacketLoss)
	if stats.PacketsRecv == 0 {
		return
	}
	dialer.emaRTT.UpdateDuration(stats.AvgRtt)
	if stats.PacketLoss == 0 {
		dialer.markSuccess()
	}
	return
}

// adds randomization to make requests less distinguishable on the network.
func randomize(d time.Duration) time.Duration {
	return time.Duration((d.Nanoseconds() / 2) + rand.Int63n(d.Nanoseconds()))
}
