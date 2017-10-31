package chained

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/withtimeout"
)

var (
	httpPingMx sync.Mutex
)

func (p *proxy) ProbePerformance() {
	log.Debugf("Actively probing performance for %v", p.Label())
	for i := 0; i < 5; i++ {
		// we vary the size of the ping request to help the BBR curve-fitting
		// logic on the server.
		kb := 50 + i*25
		// Reset BBR stats to have an up-to-date estimation after the probe
		err := p.httpPing(kb, i == 0)
		if err != nil {
			log.Errorf("Error probing %v: %v", p.Label(), err)
			return
		}
		// Sleep just a little to allow interleaving of pings for different proxies
		time.Sleep(randomize(50 * time.Millisecond))
	}
}

func (p *proxy) httpPing(kb int, resetBBR bool) error {
	// Only check one proxy at time to give ourselves the full available pipe
	httpPingMx.Lock()
	defer httpPingMx.Unlock()

	log.Debugf("Sending HTTP Ping to %v", p.Label())
	rt := &http.Transport{
		DisableKeepAlives: true,
		Dial:              p.Dial,
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
