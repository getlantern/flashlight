package chained

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/withtimeout"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/mtime"

	log "github.com/sirupsen/logrus"
)

var (
	httpPingMx sync.RWMutex
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
	log.Infof("Actively probing %v%v", p.Label(), forPerformanceString)

	elapsed := mtime.Stopwatch()
	logResult := func(succeeded bool, kb int) bool {
		successString := "failed"
		if succeeded {
			successString = "succeeded"
		}
		log.Infof("Actively probing %v with %dkb took %v and %v", p.Label(), kb, elapsed(), successString)
		return succeeded
	}

	if !forPerformance {
		// not probing for performance, just do a small ping
		err := p.httpPing(1, false)
		if err != nil {
			log.Errorf("Error probing %v: %v", p.Label(), err)
			p.MarkFailure()
			return logResult(false, 1)
		}
		p.markSuccess()
		return logResult(true, 1)
	}

	// probing for performance, do several increasingly large pings
	var kb int
	for i := 0; i < 5; i++ {
		// we vary the size of the ping request to help the BBR curve-fitting
		// logic on the server.
		kb = 50 + i*25
		// Ask the proxy to reset BBR stats to have an up-to-date estimation
		// after the probe.
		err := p.httpPing(kb, i == 0)
		if err != nil {
			log.Errorf("Error probing %v for performance: %v", p.Label(), err)
			return logResult(false, kb)
		}
		// Sleep just a little to allow interleaving of pings for different proxies
		time.Sleep(randomize(50 * time.Millisecond))
	}
	return logResult(true, kb)
}

func (p *proxy) httpPing(kb int, resetBBR bool) error {
	if kb == 1 {
		// When checking connectivity, run in parallel to reduce variables when
		// comparing proxies.
		httpPingMx.RLock()
		defer httpPingMx.RUnlock()
	} else {
		// When probing for performance, only check one proxy at time to give
		// ourselves the full available pipe.
		httpPingMx.Lock()
		defer httpPingMx.Unlock()
	}

	op := ops.Begin("probe").ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network())
	defer op.End()

	// Also include a probe_details op that's sampled but includes details like
	// the actual error.
	detailOp := ops.Begin("probe_details")
	defer detailOp.End()

	start := time.Now()
	err := p.doHttpPing(kb, resetBBR)
	delta := time.Since(start)
	if err != nil {
		atomic.AddUint64(&p.probeFailures, 1)
		atomic.AddUint64(&p.probeFailedKBs, uint64(kb))
	} else {
		atomic.AddUint64(&p.probeSuccesses, 1)
		atomic.AddUint64(&p.probeSuccessKBs, uint64(kb))
	}
	detailOp.FailIf(err)
	op.FailIf(err)
	op.Set("success", err == nil)
	op.Set("probe_kb", kb)
	op.SetMetricAvg("probe_rtt", float64(delta)/float64(time.Millisecond))
	return err
}

func (p *proxy) doHttpPing(kb int, resetBBR bool) error {
	req, e := http.NewRequest("GET", "http://ping-chained-server", nil)
	if e != nil {
		return fmt.Errorf("Could not create HTTP request: %v", e)
	}
	req.Header.Set(common.PingHeader, fmt.Sprint(kb))
	p.onRequest(req)
	if resetBBR {
		req.Header.Set("X-BBR", "clear")
	}

	_, _, err := withtimeout.Do(30*time.Second, func() (interface{}, error) {
		var dialEnd time.Time
		dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
			tk := time.NewTicker(time.Second)
			for {
				pd := p.Preconnected()
				if pd != nil {
					tk.Stop()
					pc, _, err := pd.DialContext(ctx, network, addr)
					dialEnd = time.Now()
					return pc, err
				}
				select {
				case <-ctx.Done():
					tk.Stop()
					return nil, ctx.Err()
				case <-tk.C:
					continue
				}
			}
		}

		rt := &http.Transport{
			DisableKeepAlives:     true,
			DialContext:           dial,
			ResponseHeaderTimeout: 20 * time.Second,
		}
		reqTime := time.Now()
		resp, rtErr := rt.RoundTrip(req)
		if rtErr != nil {
			return false, errors.New("Error testing dialer %s: %s", p.Addr(), rtErr)
		}
		// Note that it is updated before reading the body in hope to measure
		// more accurate RTT on the wire.
		p.updateEstRTT(time.Since(dialEnd))
		if resp.Body != nil {
			// Read the body to include this in our timing.
			defer resp.Body.Close()
			if _, copyErr := io.Copy(ioutil.Discard, resp.Body); copyErr != nil {
				return false, errors.New("Unable to read response body: %v", copyErr)
			}
		}
		log.Infof("PING through chained server at %s, status code %d", p.Addr(), resp.StatusCode)
		success := resp.StatusCode >= 200 && resp.StatusCode <= 299
		if success {
			p.collectBBRInfo(reqTime, resp)
		}
		return success, nil
	})

	return err
}
