package chained

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/withtimeout"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/mtime"
)

var (
	httpPingMx sync.Mutex
)

func (p *proxy) Probe(forPerformance bool) bool {
	forPerformanceString := ""
	if forPerformance {
		forPerformanceString = " for performance"
	}
	log.Debugf("Actively probing %v%v", p.Label(), forPerformanceString)

	elapsed := mtime.Stopwatch()
	logResult := func(succeeded bool) bool {
		forPerformanceString := ""
		if forPerformance {
			forPerformanceString = " for performance"
		}
		successString := "failed"
		if succeeded {
			successString = "succeeded"
		}
		log.Debugf("Actively probing %v%v took %v and %v", p.Label(), forPerformanceString, elapsed(), successString)
		return succeeded
	}

	if !forPerformance {
		// not probing for performance, just do a small ping
		err := p.httpPing(1, false)
		if err != nil {
			log.Errorf("Error probing %v: %v", p.Label(), err)
			p.MarkFailure()
			return logResult(false)
		}
		p.markSuccess()
		return logResult(true)
	}

	// probing for performance, do several increasingly large pings
	for i := 0; i < 5; i++ {
		// we vary the size of the ping request to help the BBR curve-fitting
		// logic on the server.
		kb := 50 + i*25
		// Reset BBR stats to have an up-to-date estimation after the probe
		err := p.httpPing(kb, i == 0)
		if err != nil {
			log.Errorf("Error probing %v for performance: %v", p.Label(), err)
			return logResult(false)
		}
		// Sleep just a little to allow interleaving of pings for different proxies
		time.Sleep(randomize(50 * time.Millisecond))
	}
	return logResult(true)
}

func (p *proxy) httpPing(kb int, resetBBR bool) error {
	// Only check one proxy at time to give ourselves the full available pipe
	httpPingMx.Lock()
	defer httpPingMx.Unlock()

	op := ops.Begin("probe").ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network())
	defer op.End()

	start := time.Now()
	err := op.FailIf(p.doHttpPing(kb, resetBBR))
	delta := time.Now().Sub(start)
	op.Set("success", err == nil)
	op.SetMetricAvg("probe_rtt", float64(delta)/float64(time.Millisecond))
	log.Debugf("Probe took %v", delta)

	return err
}

func (p *proxy) doHttpPing(kb int, resetBBR bool) error {
	log.Debugf("Sending HTTP Ping to %v", p.Label())
	rt := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := p.doDialServer(ctx, p)
			if err != nil {
				return nil, err
			}
			pd := p.newPreconnected(conn)
			pc, _, err := pd.DialContext(ctx, network, addr)
			return pc, err
		},
		ResponseHeaderTimeout: 20 * time.Second,
	}

	req, err := http.NewRequest("GET", "http://ping-chained-server", nil)
	if err != nil {
		return fmt.Errorf("Could not create HTTP request: %v", err)
	}
	req.Header.Set(common.PingHeader, fmt.Sprint(kb))
	p.onRequest(req)
	if resetBBR {
		req.Header.Set("X-BBR", "clear")
	}

	_, _, err = withtimeout.Do(30*time.Second, func() (interface{}, error) {
		reqTime := time.Now()
		resp, rtErr := rt.RoundTrip(req)
		if rtErr != nil {
			return false, fmt.Errorf("Error testing dialer %s: %s", p.Addr(), rtErr)
		}
		if resp.Body != nil {
			// Read the body to include this in our timing.
			defer resp.Body.Close()
			if _, copyErr := io.Copy(ioutil.Discard, resp.Body); copyErr != nil {
				return false, fmt.Errorf("Unable to read response body: %v", copyErr)
			}
		}
		log.Tracef("PING through chained server at %s, status code %d", p.Addr(), resp.StatusCode)
		success := resp.StatusCode >= 200 && resp.StatusCode <= 299
		if success {
			p.collectBBRInfo(reqTime, resp)
		}
		return success, nil
	})

	return err
}
