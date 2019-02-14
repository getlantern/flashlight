package chained

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/mtime"
)

var (
	// Probes determines how many times to probe on each call to Probe() unless
	// it's for performance. Should be at least 3 to generate enough packets,
	// as the censor may allow the first a few packets but block the rest.
	Probes = 3

	// PerformanceProbes determines how many times to probe for performance on
	// each call to Probe()
	PerformanceProbes = 5

	// BasePerformanceProbeKB is the minimum number of KB to request from ping
	// endpoint when probing for performance
	BasePerformanceProbeKB = 50
)

func (p *proxy) ProbeStats() (successes uint64, successKBs uint64, failures uint64, failedKBs uint64) {
	return atomic.LoadUint64(&p.probeSuccesses), atomic.LoadUint64(&p.probeSuccessKBs),
		atomic.LoadUint64(&p.probeFailures), atomic.LoadUint64(&p.probeFailedKBs)
}

func (p *proxy) Probe(forPerformance bool) bool {
	forPerformanceString := ""
	if forPerformance {
		forPerformanceString = " for performance"
	}
	log.Debugf("Actively probing %v%v", p.Label(), forPerformanceString)

	elapsed := mtime.Stopwatch()
	logResult := func(err error) bool {
		result := "succeeded"
		if err != nil {
			result = "failed: " + err.Error()
		}
		log.Debugf("Actively probing %v%v took %v and %v", p.Label(), forPerformanceString, elapsed(), result)
		return err == nil
	}

	err := p.httpPing(forPerformance)
	if err != nil {
		p.MarkFailure()
	} else {
		p.markSuccess()
	}
	return logResult(err)
}

func (p *proxy) httpPing(forPerformance bool) error {
	op := ops.Begin("probe").ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network(), p.multiplexed)
	defer op.End()

	// Also include a probe_details op that's sampled but includes details like
	// the actual error.
	detailOp := ops.Begin("probe_details")
	defer detailOp.End()

	var dialEnd time.Time
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		pc, _, err := p.DialContext(ctx, network, addr)
		dialEnd = time.Now()
		return pc, err
	}
	rt := &http.Transport{
		DisableKeepAlives:     true,
		DialContext:           dial,
		ResponseHeaderTimeout: 20 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	probes := Probes
	if forPerformance {
		probes = PerformanceProbes
	}
	var err error
	var totalKB, i int
	var sumOfRTT int64
	for ; i < probes; i++ {
		kb := 1
		resetBBR := false
		if forPerformance {
			// Probing for performance, do several increasingly large pings.
			// We vary the size of the ping request to help the BBR curve-fitting
			// logic on the server.
			kb = BasePerformanceProbeKB + i*25
			// Ask the proxy to reset BBR stats to have an up-to-date estimation
			// after the probe.
			resetBBR = i == 0
		}
		totalKB += kb

		start := time.Now()
		tofb, err := p.doHttpPing(ctx, rt, kb, resetBBR)
		// exclude the time to dial proxy to have more accurate RTT
		if start.Before(dialEnd) {
			start = dialEnd
		}
		sumOfRTT += time.Since(start).Nanoseconds()

		if err != nil {
			atomic.AddUint64(&p.probeFailures, 1)
			atomic.AddUint64(&p.probeFailedKBs, uint64(kb))
		} else {
			atomic.AddUint64(&p.probeSuccesses, 1)
			atomic.AddUint64(&p.probeSuccessKBs, uint64(kb))
			if i == 0 {
				p.updateEstRTT(tofb.Sub(dialEnd))
			}
		}
		if err != nil {
			break
		}
	}
	detailOp.FailIf(err)
	op.FailIf(err)
	op.Set("success", err == nil)
	op.Set("probe_kb", totalKB)
	avgRTT := sumOfRTT / int64(i+1)
	op.SetMetricAvg("probe_rtt", float64(avgRTT)/float64(time.Millisecond))
	return err
}

func (p *proxy) doHttpPing(ctx context.Context, rt http.RoundTripper, kb int, resetBBR bool) (tofb time.Time, err error) {
	deadline, _ := ctx.Deadline()
	req, e := http.NewRequest("GET", "http://ping-chained-server", nil)
	if e != nil {
		return deadline, fmt.Errorf("Could not create HTTP request: %v", e)
	}
	req.Header.Set(common.PingHeader, fmt.Sprint(kb))
	p.onRequest(req)
	if resetBBR {
		req.Header.Set("X-BBR", "clear")
	}

	reqTime := time.Now()
	resp, rtErr := rt.RoundTrip(req.WithContext(ctx))
	if rtErr != nil {
		return deadline, errors.New("Error testing dialer %s: %s", p.Addr(), rtErr)
	}
	// Time of first byte. Note that it is updated before reading the body in
	// hope to measure more accurate RTT on the wire.
	tofb = time.Now()
	if resp.Body != nil {
		// Read the body to include this in our timing.
		defer resp.Body.Close()
		if _, copyErr := io.Copy(ioutil.Discard, resp.Body); copyErr != nil {
			return deadline, errors.New("Unable to read response body: %v", copyErr)
		}
	}
	log.Tracef("PING through chained server at %s, status code %d", p.Addr(), resp.StatusCode)
	if sameStatusCodeClass(http.StatusOK, resp.StatusCode) {
		p.collectBBRInfo(reqTime, resp)
	}
	return tofb, nil
}
