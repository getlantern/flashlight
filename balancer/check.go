package balancer

import (
	"math/rand"
	"time"
)

func (b *Balancer) runChecks() {
	checkInterval := b.minCheckInterval
	checkTimer := time.NewTimer(0) // check immediately on Start

	for {
		select {
		case <-b.closeCh:
			log.Trace("Balancer stopped")
			b.stopStats <- true
			b.mu.Lock()
			oldDialers := b.dialers
			b.dialers.dialers = nil
			b.mu.Unlock()
			for _, d := range oldDialers.dialers {
				d.Stop()
			}
			checkTimer.Stop()
			return
		case <-b.resetCheckCh:
			checkInterval = b.minCheckInterval
			checkTimer.Reset(checkInterval)
		case <-checkTimer.C:
			// Obtain check data and then run checks for all dialers using the same
			// check data. This ensures that if the specific checks vary over time,
			// we get an apples to apples comparison across all dialers.
			checkData := b.checkData()
			b.mu.RLock()
			dialers := make([]*dialer, 0, len(b.dialers.dialers))
			for _, d := range b.dialers.dialers {
				if d.Check == nil {
					log.Errorf("No check function provided for dialer %s, not checking", d.Label)
					continue
				}
				dialers = append(dialers, d)
			}
			b.mu.RUnlock()
			log.Debugf("Checking %d dialers using %v", len(dialers), checkData)
			for _, dialer := range dialers {
				dialer.check(checkData)
			}
			// Exponentially back off checkInterval capped to MaxCheckInterval
			checkInterval *= 2
			if checkInterval > b.maxCheckInterval {
				checkInterval = b.maxCheckInterval
			}
			checkTimer.Reset(randomize(checkInterval))
			log.Debugf("Finished checking dialers")
		}
	}
}

func (b *Balancer) forceRecheck() {
	select {
	case b.resetCheckCh <- true:
		log.Debug("Forced recheck")
	default:
		// Pending reset, ignore subsequent request
	}
}

// adds randomization to make requests less distinguishable on the network.
func randomize(d time.Duration) time.Duration {
	return time.Duration((d.Nanoseconds() / 2) + rand.Int63n(d.Nanoseconds()))
}
