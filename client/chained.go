package client

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/bbr"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/netx"
	"github.com/getlantern/withtimeout"
)

var (
	// Close connections idle for a period to avoid dangling connections. 45 seconds
	// is long enough to avoid interrupt normal connections but shorter than the
	// idle timeout on the server to avoid running into closed connection problems.
	// 45 seconds is also longer than the MaxIdleTime on our http.Transport, so it
	// doesn't interfere with that.
	idleTimeout = 45 * time.Second
)

// ChainedDialer creates a *balancer.Dialer backed by a chained server.
func ChainedDialer(name string, si *chained.ChainedServerInfo, deviceID string, proTokenGetter func() string) (*balancer.Dialer, error) {
	s, err := newServer(name, si)
	if err != nil {
		return nil, err
	}
	return s.dialer(deviceID, proTokenGetter)
}

type chainedServer struct {
	chained.Proxy
}

func newServer(name string, _si *chained.ChainedServerInfo) (*chainedServer, error) {
	// Copy server info to allow modifying
	si := &chained.ChainedServerInfo{}
	*si = *_si
	// Backwards-compatibility for clients that still have old obfs4
	// configurations on disk.
	if si.PluggableTransport == "obfs4-tcp" {
		si.PluggableTransport = "obfs4"
	}

	p, err := chained.CreateProxy(name, si)
	if err != nil {
		return nil, err
	}
	return &chainedServer{p}, nil
}

func (s *chainedServer) dialer(deviceID string, proTokenGetter func() string) (*balancer.Dialer, error) {
	ccfg := chained.Config{
		DialServer: s.Proxy.DialServer,
		Label:      s.Label(),
		OnRequest: func(req *http.Request) {
			s.attachHeaders(req, deviceID, proTokenGetter)
		},
		OnFinish: func(op *ops.Op) {
			op.ChainedProxy(s.Addr(), s.Proxy.Protocol(), s.Proxy.Network())
		},
	}
	d := chained.NewDialer(ccfg)
	return &balancer.Dialer{
		Label:   s.Label(),
		Trusted: s.Trusted(),
		DialFN: func(network, addr string) (net.Conn, error) {
			var conn net.Conn
			var err error

			op := ops.Begin("dial_for_balancer").ProxyType(ops.ProxyChained).ProxyAddr(s.Addr())
			defer op.End()

			if addr == s.Addr() {
				// Check if we are trying to connect to our own server and bypass proxying if so
				// This accounts for the case w/ multiple instances of Lantern running on mobile
				// Whenever full-device VPN mode is enabled, we need to make sure we ignore proxy
				// requests from the first instance.
				log.Debugf("Attempted to dial ourselves. Dialing directly to %s instead", addr)
				conn, err = netx.DialTimeout("tcp", addr, 1*time.Minute)
			} else {
				conn, err = d(network, addr)
				if err == nil {
					// Yeah any site visited through Lantern can be a check target, but
					// only check it if the dial was successful.
					balancer.AddCheckTarget(addr)
				}
			}

			if err != nil {
				return nil, op.FailIf(err)
			}
			conn = idletiming.Conn(conn, idleTimeout, func() {
				log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), idleTimeout)
			})
			return conn, nil
		},
		Check: func(checkData interface{}, onFailure func(string)) (bool, time.Duration) {
			return s.check(d, checkData.([]string), deviceID, proTokenGetter, onFailure)
		},
	}, nil
}

func (s *chainedServer) attachHeaders(req *http.Request, deviceID string, proTokenGetter func() string) {
	s.Proxy.AdaptRequest(req)
	req.Header.Set(common.DeviceIdHeader, deviceID)
	req.Header.Set(common.VersionHeader, common.Version)
	if token := proTokenGetter(); token != "" {
		req.Header.Set(common.ProTokenHeader, token)
	}
}

// check pings the 10 most popular sites in the user's history
func (s *chainedServer) check(dial func(string, string) (net.Conn, error),
	urls []string, deviceID string,
	proTokenGetter func() string,
	onFailure func(string)) (bool, time.Duration) {
	rt := &http.Transport{
		DisableKeepAlives: true,
		Dial:              dial,
	}

	allPassed := int32(1)
	totalLatency := int64(0)
	var wg sync.WaitGroup
	wg.Add(len(urls))
	for _, _url := range urls {
		url := _url
		ops.Go(func() {
			passed := s.doCheck(url, &totalLatency, rt, deviceID, proTokenGetter, onFailure)
			if !passed {
				atomic.StoreInt32(&allPassed, 0)
			}
			wg.Done()
		})
	}
	wg.Wait()

	return atomic.LoadInt32(&allPassed) == 1, time.Duration(atomic.LoadInt64(&totalLatency))
}

func (s *chainedServer) doCheck(url string,
	totalLatency *int64,
	rt *http.Transport,
	deviceID string,
	proTokenGetter func() string,
	onFailure func(string)) bool {
	start := time.Now()
	// We ping the URLs through the proxy to get timings
	req, err := http.NewRequest("GET", "http://ping-chained-server", nil)
	if err != nil {
		log.Errorf("Could not create HTTP request: %v", err)
		return false
	}
	req.Header.Set(common.PingURLHeader, url)
	// We set X-Lantern-Ping in case we're hitting an old http-server that
	// doesn't support pinging URLs.
	req.Header.Set(common.PingHeader, "small")

	checkedURL := url
	s.attachHeaders(req, deviceID, proTokenGetter)
	ok, timedOut, _ := withtimeout.Do(10*time.Second, func() (interface{}, error) {
		resp, err := rt.RoundTrip(req)
		if err != nil {
			log.Debugf("Error testing dialer %s to %s: %s", s.Addr(), checkedURL, err)
			return false, nil
		}
		if resp.Body != nil {
			// Read the body to include this in our timing.
			// Note - for bandwidth saving reasons, the server may not send the body
			// but if it does, we'll read it.
			defer resp.Body.Close()
			_, err = io.Copy(ioutil.Discard, resp.Body)
			if err != nil {
				return false, fmt.Errorf("Unable to read response body: %v", err)
			}
		}
		log.Tracef("PING %s through chained server at %s, status code %d", url, s.Addr(), resp.StatusCode)
		success := resp.StatusCode >= 200 && resp.StatusCode <= 299
		if success {
			bbr.OnResponse(resp)
			delta := int64(time.Now().Sub(start))
			atomic.AddInt64(totalLatency, delta)
		} else {
			onFailure(url)
		}
		return success, nil
	})
	if timedOut || !ok.(bool) {
		if timedOut {
			log.Errorf("Timed out checking %v", s.Label())
		}
		return false
	}
	return true
}
