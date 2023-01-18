// +build linux

package bbr

import (
	"sync/atomic"
	"time"
)

func (bm *middleware) ProbeUpstream(url string) {
	for {
		bm.probeUpstream(url)
		time.Sleep(10 * time.Minute)
	}
}

func (bm *middleware) probeUpstream(url string) {
	newUpstreamABE := doProbeUpstream(url)
	if newUpstreamABE > upstreamABEUnknown {
		log.Tracef("Setting upstream ABE to %v", newUpstreamABE)
		bm.setUpstreamABE(newUpstreamABE)
	}
}

func (bm *middleware) setUpstreamABE(upstream float64) {
	atomic.StoreUint64(&bm.upstreamABE, uint64(upstream*1000))
}

func (bm *middleware) getUpstreamABE() float64 {
	return float64(atomic.LoadUint64(&bm.upstreamABE)) / 1000
}
