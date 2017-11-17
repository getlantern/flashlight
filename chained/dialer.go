package chained

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/mtime"
)

const (
	minCheckInterval = 10 * time.Second
	maxCheckInterval = 15 * time.Minute
	minCheckTimeout  = 1 * time.Second
	maxCheckTimeout  = 10 * time.Second
)

var (
	// Close connections idle for a period to avoid dangling connections. 45
	// seconds is long enough to avoid interrupt normal connections but shorter
	// than the idle timeout on the server to avoid running into closed connection
	// problems. 45 seconds is also longer than the MaxIdleTime on our
	// http.Transport, so it doesn't interfere with that.
	IdleTimeout = 45 * time.Second

	// errUpstream is an error that indicates there was a problem upstream of a
	// proxy. Such errors are not counted as failures but do allow failover to
	// other proxies.
	errUpstream = errors.New("Upstream error")
)

func (p *proxy) runConnectivityChecks() {
	// Periodically check our connectivity.
	// With a 15 minute period, Lantern running 8 hours a day for 30 days and 148
	// bytes for a TCP connection setup and teardown, this check will consume
	// approximately 138 KB per month per proxy.
	checkInterval := minCheckInterval
	timer := time.NewTimer(checkInterval)

	ops.Go(func() {
		for {
			timer.Reset(randomize(checkInterval))
			select {
			case <-timer.C:
				log.Debugf("Checking %v", p.Label())
				if p.CheckConnectivity() {
					// On success, don't bother rechecking anytime soon
					checkInterval = maxCheckInterval
				} else {
					// Exponentially back off while we're still failing
					checkInterval *= 2
					if checkInterval > maxCheckInterval {
						checkInterval = maxCheckInterval
					}
				}
			case <-p.forceRecheckCh:
				log.Debugf("Forcing recheck for %v", p.Label())
				checkInterval = minCheckInterval
			case <-p.closeCh:
				log.Tracef("Dialer %v stopped", p.Label())
				timer.Stop()
				return
			}
		}
	})
}

func (p *proxy) CheckConnectivity() bool {
	elapsed := mtime.Stopwatch()
	_, err := p.DialServer()
	log.Debugf("Checking %v took %v, err: %v", p.Label(), elapsed(), err)
	if err == nil {
		p.markSuccess()
		return true
	}
	p.markFailure()
	return false
	// We intentionally don't close the connection and instead let the
	// server's idle timeout handle it to make this less fingerprintable.
}

func randomize(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int63n(int64(d)))
}

func (p *proxy) Stop() {
	log.Tracef("Stopping dialer %s", p.Label())
	// don't bother waiting for proxy to stop
	go func() {
		p.closeCh <- true
	}()
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
	// To avoid turbulence when network glitches, treat proxies with a small
	// amount failures as succeeding.
	// TODO: OTOH, when the proxy just recovered from failing, should wait for
	// a few successes to consider it as succeeding.
	return p.ConsecSuccesses()-p.ConsecFailures() > -5 &&
		p.consecRWSuccesses.Get() > -5
}

// Dial is a net.Dial-compatible function.
func (p *proxy) Dial(network, addr string) (net.Conn, error) {
	return p.DialContext(context.Background(), network, addr)
}

// DialContext dials using provided context
func (p *proxy) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := p.doDial(ctx, network, addr)
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

func (p *proxy) doDial(ctx context.Context, network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error

	op := ops.Begin("dial_for_balancer").ProxyType(ops.ProxyChained).ProxyAddr(p.addr)
	defer op.End()

	conn, err = p.dialInternal(ctx, network, addr)

	if err != nil {
		return nil, op.FailIf(err)
	}
	conn = idletiming.Conn(conn, IdleTimeout, func() {
		log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), IdleTimeout)
	})
	return conn, nil
}

func (p *proxy) dialInternal(ctx context.Context, network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error
	chDone := make(chan bool)
	go func() {
		conn, err = p.doDialInternal(network, addr)
		chDone <- true
	}()
	select {
	case <-chDone:
		return conn, err
	case <-ctx.Done():
		go func() {
			<-chDone
			if err == nil {
				log.Debugf("Connection to %s established too late, closing", addr)
				conn.Close()
			}
		}()
		return nil, ctx.Err()
	}
}

func (p *proxy) doDialInternal(network, addr string) (net.Conn, error) {
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
	return p.withRateTracking(conn, addr), nil
}

func (p *proxy) onRequest(req *http.Request) {
	p.AdaptRequest(req)
	req.Header.Set(common.DeviceIdHeader, p.deviceID)
	req.Header.Set(common.VersionHeader, common.Version)
	if token := p.proToken(); token != "" {
		req.Header.Set(common.ProTokenHeader, token)
	}
	// Request BBR metrics
	req.Header.Set("X-BBR", "y")
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
	p.collectBBRInfo(reqTime, resp)
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
