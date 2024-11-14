package dialer

import (
	"context"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	bandit "github.com/alextanhongpin/go-bandit"
)

// BanditDialer is responsible for continually choosing the optimized dialer.
type BanditDialer struct {
	dialers []ProxyDialer
	bandit  *bandit.EpsilonGreedy
	opts    *Options
}

// NewBandit creates a new bandit given the available dialers and options with
// callbacks to be called when a dialer is selected, an error occurs, etc.
func NewBandit(opts *Options) (Dialer, error) {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(ProxyDialer) {}
	}

	dialers := opts.Dialers
	log.Debugf("Creating bandit with %d dialers", len(dialers))
	b, err := bandit.NewEpsilonGreedy(0.1, nil, nil)
	if err != nil {
		log.Errorf("unable to create bandit: %v", err)
		return nil, err
	}

	if err := b.Init(len(dialers)); err != nil {
		log.Errorf("unable to initialize bandit: %v", err)
		return nil, err
	}
	dialer := &BanditDialer{
		dialers: dialers,
		bandit:  b,
		opts:    opts,
	}

	return dialer, nil
}

func (bd *BanditDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	deadline, _ := ctx.Deadline()
	log.Debugf("bandit::DialContext::time remaining: %v", time.Until(deadline))
	// We can not create a multi-armed bandit with no arms.
	if len(bd.dialers) == 0 {
		return nil, log.Error("Cannot dial with no dialers")
	}

	start := time.Now()
	d, chosenArm := bd.chooseDialerForDomain(network, addr)

	// We have to be careful here about virtual, multiplexed connections, as the
	// initial TCP dial will have different performance characteristics than the
	// subsequent virtual connection dials.
	log.Debugf("bandit::dialer %d: %s at %v", chosenArm, d.Label(), d.Addr())
	conn, failedUpstream, err := d.DialContext(ctx, network, addr)
	if err != nil {
		hasSucceeding := hasSucceedingDialer(bd.dialers)
		bd.opts.OnError(err, hasSucceeding)

		if !failedUpstream {
			log.Errorf("Dialer %v failed in %v seconds: %v", d.Name(), time.Since(start).Seconds(), err)
			if err := bd.bandit.Update(chosenArm, 0); err != nil {
				log.Errorf("unable to update bandit: %v", err)
			}
		} else {
			log.Debugf("Dialer %v failed upstream...", d.Name())
			// This can happen, for example, if the upstream server is down, or
			// if the DNS resolves to localhost, for example. It is also possible
			// that the proxy is blacklisted by upstream sites for some reason,
			// so we have to choose some reasonable value.
			if err := bd.bandit.Update(chosenArm, 0.00005); err != nil {
				log.Errorf("unable to update bandit: %v", err)
			}
		}
		return nil, err
	}
	log.Debugf("Dialer %v dialed in %v seconds", d.Name(), time.Since(start).Seconds())
	// We don't give any special reward for a successful dial here and just rely on
	// the normalized raw throughput to determine the reward. This is because the
	// reward system takes into account how many tries there have been for a given
	// "arm", so giving a reward here would be double-counting.

	// Tell the dialer to update the bandit with it's throughput after 5 seconds.
	var dataRecv atomic.Uint64
	dt := newDataTrackingConn(conn, &dataRecv)
	time.AfterFunc(secondsForSample*time.Second, func() {
		speed := normalizeReceiveSpeed(dataRecv.Load())
		//log.Debugf("Dialer %v received %v bytes in %v seconds, normalized speed: %v", d.Name(), dt.dataRecv, secondsForSample, speed)
		if err := bd.bandit.Update(chosenArm, speed); err != nil {
			log.Errorf("unable to update bandit: %v", err)
		}
	})

	bd.opts.OnSuccess(d)
	return dt, err
}

func (o *BanditDialer) chooseDialerForDomain(network, addr string) (ProxyDialer, int) {
	// Loop through the number of dialers we have and select the one that is best
	// for the given domain.
	chosenArm := o.bandit.SelectArm(rand.Float64())
	var d ProxyDialer
	notAllFailing := hasNotFailing(o.dialers)
	for i := 0; i < (len(o.dialers) * 2); i++ {
		d = o.dialers[chosenArm]
		if (d.ConsecFailures() > 0 && notAllFailing) || !d.SupportsAddr(network, addr) {
			// If the chosen dialer has consecutive failures and there are other
			// dialers that are succeeding, we should choose a different dialer.
			//
			// If the chosen dialer does not support the address, we should also
			// choose a different dialer.
			chosenArm = differentArm(chosenArm, len(o.dialers))
			continue
		}
		break
	}
	return d, chosenArm
}

// Choose a different arm than the one we already have, if possible.
func differentArm(existingArm, numDialers int) int {
	// This selects a new arm randomly, which is preferable to just choosing
	// the next one in the list because that will always be the next dialer
	// after whatever dialer is currently best.
	for i := 0; i < 20; i++ {
		newArm := rand.Intn(numDialers)
		if newArm != existingArm {
			return newArm
		}
	}

	// If random selection doesn't work, just choose the next one.
	return (existingArm + 1) % numDialers
}

const secondsForSample = 6

// A reasonable upper bound for the top expected bytes to receive per second.
// Anything over this will be normalized to over 1.
const topExpectedBps = 125000

func normalizeReceiveSpeed(dataRecv uint64) float64 {
	// Record the bytes in relation to the top expected speed.
	return (float64(dataRecv) / secondsForSample) / topExpectedBps
}

func (o *BanditDialer) Close() {
	log.Debug("Closing all dialers")
	for _, d := range o.dialers {
		d.Stop()
	}
}

func newDataTrackingConn(conn net.Conn, dataRecv *atomic.Uint64) *dataTrackingConn {
	return &dataTrackingConn{
		Conn:     conn,
		dataRecv: dataRecv,
	}
}

type dataTrackingConn struct {
	net.Conn
	dataRecv *atomic.Uint64
}

func (c *dataTrackingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.dataRecv.Add(uint64(n))
	return n, err
}
