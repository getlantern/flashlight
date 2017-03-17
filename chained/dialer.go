package chained

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/netx"
)

const (
	minCheckInterval = 10 * time.Second
	maxCheckInterval = 15 * time.Minute
	minCheckTimeout  = 1 * time.Second
	maxCheckTimeout  = 10 * time.Second
)

var (
	// Close connections idle for a period to avoid dangling connections. 45 seconds
	// is long enough to avoid interrupt normal connections but shorter than the
	// idle timeout on the server to avoid running into closed connection problems.
	// 45 seconds is also longer than the MaxIdleTime on our http.Transport, so it
	// doesn't interfere with that.
	IdleTimeout = 45 * time.Second

	// errUpstream is an error that indicates there was a problem upstream of a
	// proxy. Such errors are not counted as failures but do allow failover to
	// other proxies.
	errUpstream = errors.New("Upstream error")
)

func (p *proxy) Start() {
	// Periodically check our connectivity.
	// With a 15 minute period, Lantern running 8 hours a day for 30 days and 148
	// bytes for a TCP connection setup and teardown, this check will consume
	// approximately 138 KB per month per proxy.
	checkInterval := maxCheckInterval
	timer := time.NewTimer(checkInterval)

	ops.Go(func() {
		for {
			select {
			case <-timer.C:
				log.Debugf("Checking %v", p.Label())
				timeout := p.emaLatencyLongTerm.GetDuration() * 2
				if timeout < minCheckTimeout {
					timeout = minCheckTimeout
				} else if timeout > maxCheckTimeout {
					timeout = maxCheckTimeout
				}
				conn, _, err := p.dialTCP(timeout)
				if err == nil {
					p.markSuccess()
					conn.Close()
					// On success, don't bother rechecking anytime soon
					checkInterval = maxCheckInterval
				} else {
					p.markFailure()
					// Exponentially back off while we're still failing
					checkInterval *= 2
					if checkInterval > maxCheckInterval {
						checkInterval = maxCheckInterval
					}
				}
				timer.Reset(checkInterval)
			case <-p.forceRecheckCh:
				log.Debugf("Forcing recheck for %v", p.Label())
				checkInterval := minCheckInterval
				timer.Reset(checkInterval)
			case <-p.closeCh:
				log.Tracef("Dialer %v stopped", p.Label())
				timer.Stop()
				return
			}
		}
	})
}

func (p *proxy) Stop() {
	log.Tracef("Stopping dialer %s", p.Label())
	p.closeCh <- true
}

func (p *proxy) Attempts() int64 {
	return atomic.LoadInt64(&p.attempts)
}

func (p *proxy) Successes() int64 {
	return atomic.LoadInt64(&p.successes)
}

func (p *proxy) ConsecSuccesses() int64 {
	return atomic.LoadInt64(&p.consecSuccesses)
}

func (p *proxy) Failures() int64 {
	return atomic.LoadInt64(&p.failures)
}

func (p *proxy) ConsecFailures() int64 {
	return atomic.LoadInt64(&p.consecFailures)
}

func (p *proxy) Succeeding() bool {
	return p.ConsecSuccesses()-p.ConsecFailures() > 0
}

// Dial is a net.Dial-compatible function.
func (p *proxy) Dial(network, addr string) (net.Conn, error) {
	conn, err := p.doDial(network, addr)
	if err != nil {
		if err != errUpstream {
			p.markFailure()
		}
	} else {
		p.markSuccess()
	}
	return conn, err
}

func (p *proxy) markSuccess() {
	atomic.AddInt64(&p.attempts, 1)
	atomic.AddInt64(&p.successes, 1)
	newCS := atomic.AddInt64(&p.consecSuccesses, 1)
	log.Tracef("Dialer %s consecutive successes: %d -> %d", p.Label(), newCS-1, newCS)
	// only when state is changing
	if newCS <= 2 {
		atomic.StoreInt64(&p.consecFailures, 0)
	}
}

func (p *proxy) markFailure() {
	if p.doMarkFailure() == 1 {
		// On new failure, force recheck
		p.forceRecheck()
	}
}

func (p *proxy) doMarkFailure() int64 {
	atomic.AddInt64(&p.attempts, 1)
	atomic.AddInt64(&p.failures, 1)
	atomic.StoreInt64(&p.consecSuccesses, 0)
	newCF := atomic.AddInt64(&p.consecFailures, 1)
	log.Tracef("Dialer %s consecutive failures: %d -> %d", p.Label(), newCF-1, newCF)
	return newCF
}

func (p *proxy) doDial(network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error

	op := ops.Begin("dial_for_balancer").ProxyType(ops.ProxyChained).ProxyAddr(p.addr)
	defer op.End()

	if addr == p.addr {
		// Check if we are trying to connect to our own server and bypass proxying if so
		// This accounts for the case w/ multiple instances of Lantern running on mobile
		// Whenever full-device VPN mode is enabled, we need to make sure we ignore proxy
		// requests from the first instance.
		log.Debugf("Attempted to dial ourselves. Dialing directly to %s instead", addr)
		conn, err = netx.DialTimeout("tcp", addr, 1*time.Minute)
	} else {
		conn, err = p.dialInternal(network, addr)
	}

	if err != nil {
		return nil, op.FailIf(err)
	}
	conn = idletiming.Conn(conn, IdleTimeout, func() {
		log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), IdleTimeout)
	})
	return conn, nil
}

func (p *proxy) dialInternal(network, addr string) (net.Conn, error) {
	conn, err := p.DialServer()
	if err != nil {
		return nil, errors.New("Unable to dial server %v: %s", p.Label(), err)
	}
	// Look for our special hacked "connect" transport used to signal
	// that we should send a CONNECT request and tunnel all traffic through
	// that.
	switch network {
	case "connect":
		log.Tracef("Sending CONNECT request")
		err = p.sendCONNECT(addr, conn)
	case "persistent":
		log.Tracef("Sending GET request to establish persistent HTTP connection")
		err = p.initPersistentConnection(addr, conn)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return withRateTracking(conn, addr, p.onFinish), nil
}

func (p *proxy) onRequest(req *http.Request) {
	p.AdaptRequest(req)
	req.Header.Set("X-Lantern-Device-Id", p.deviceID)
	if token := p.proToken(); token != "" {
		req.Header.Set("X-Lantern-Pro-Token", token)
	}
}

func (p *proxy) onFinish(op *ops.Op) {
	op.ChainedProxy(p.addr, p.protocol, p.network)
}

func (p *proxy) sendCONNECT(addr string, conn net.Conn) error {
	reqTime := time.Now()
	req, err := p.buildCONNECTRequest(addr)
	if err != nil {
		return fmt.Errorf("Unable to construct CONNECT request: %s", err)
	}
	err = req.Write(conn)
	if err != nil {
		return fmt.Errorf("Unable to write CONNECT request: %s", err)
	}

	r := bufio.NewReader(conn)
	err = p.checkCONNECTResponse(r, req, reqTime)
	return err
}

func (p *proxy) buildCONNECTRequest(addr string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodConnect, "/", nil)
	if err != nil {
		return nil, err
	}
	req.URL = &url.URL{
		Host: addr,
	}
	req.Host = addr
	p.onRequest(req)
	// Request BBR metrics
	bbrOption := "y"
	if p.ShouldResetBBR() {
		bbrOption = "clear"
	}
	req.Header.Set("X-BBR", bbrOption)
	return req, nil
}

func (p *proxy) checkCONNECTResponse(r *bufio.Reader, req *http.Request, reqTime time.Time) error {
	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return fmt.Errorf("Error reading CONNECT response: %s", err)
	}
	if !sameStatusCodeClass(http.StatusOK, resp.StatusCode) {
		var body []byte
		if resp.Body != nil {
			defer resp.Body.Close()
			body, _ = ioutil.ReadAll(resp.Body)
		}
		log.Errorf("Bad status code on CONNECT response %d: %v", resp.StatusCode, string(body))
		return errUpstream
	}
	p.CollectBBRInfo(reqTime, resp)
	bandwidth.Track(resp)
	return nil
}

func sameStatusCodeClass(statusCode1 int, statusCode2 int) bool {
	// HTTP response status code "classes" come in ranges of 100.
	const classRange = 100
	// These are all integers, so division truncates.
	return statusCode1/classRange == statusCode2/classRange
}

func (p *proxy) initPersistentConnection(addr string, conn net.Conn) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v", addr), nil)
	if err != nil {
		return err
	}
	p.onRequest(req)
	req.Header.Set("X-Lantern-Persistent", "true")
	writeErr := req.Write(conn)
	if writeErr != nil {
		return fmt.Errorf("Unable to write initial request: %v", writeErr)
	}

	return nil
}
