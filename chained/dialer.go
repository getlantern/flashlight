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

	"github.com/mitchellh/go-server-timing"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	gp "github.com/getlantern/proxy"
	log "github.com/sirupsen/logrus"
)

const (
	minCheckInterval      = 10 * time.Second
	maxCheckInterval      = 15 * time.Minute
	dialCoreCheckInterval = 30 * time.Second

	connect    = "connect"
	persistent = "persistent"
)

type proxyConnection struct {
	*proxy
	conn      serverConn
	expiresAt time.Time
}

var (
	// IdleTimeout closes connections idle for a period to avoid dangling connections. 45
	// seconds is long enough to avoid interrupt normal connections but shorter
	// than the idle timeout on the server to avoid running into closed connection
	// problems. 45 seconds is also longer than the MaxIdleTime on our
	// http.Transport, so it doesn't interfere with that.
	IdleTimeout = 45 * time.Second
	// set the preconnected dialer to expire before we hit the idle timeout
	expireTimeout = IdleTimeout - 1*time.Second

	// errUpstream is an error that indicates there was a problem upstream of a
	// proxy. Such errors are not counted as failures but do allow failover to
	// other proxies.
	errUpstream = errors.New("Upstream error")
)

// Periodically call doDialCore to make sure we're recording updated latencies.
func (p *proxy) checkCoreDials() {
	timer := time.NewTimer(0)

	ops.Go(func() {
		log.Debugf("Will probe core dials to %v", p.Label())
		for {
			timer.Reset(randomize(dialCoreCheckInterval))
			select {
			case <-timer.C:
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dialCoreCheckInterval/2))
				conn, _, err := p.doDialCore(ctx)
				if err == nil {
					conn.Close()
				}
				cancel()
			case <-p.closeCh:
				log.Debugf("Dialer %v stopped", p.Label())
				timer.Stop()
				return
			}
		}
	})
}

func randomize(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int63n(int64(d)))
}

func (p *proxy) Stop() {
	log.Debugf("Stopping dialer %s", p.Label())
	p.closeOnce.Do(func() {
		close(p.closeCh)
	})
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
	return p.ConsecSuccesses()-p.ConsecFailures() > 0 &&
		p.consecRWSuccesses.Get() > 0
}

func (p *proxy) DataSent() uint64 {
	return atomic.LoadUint64(&p.dataSent)
}

func (p *proxy) DataRecv() uint64 {
	return atomic.LoadUint64(&p.dataRecv)
}

func (p *proxy) processPreconnects(initPreconnect int) {
	// Fill pool to initial size
	for i := 0; i < initPreconnect; i++ {
		p.Preconnect()
	}
	// Preconnect in parallel
	maxPreconnects := cap(p.preconnects)
	for i := 0; i < maxPreconnects; i++ {
		go p.doPreconnect()
	}
}

func (p *proxy) doPreconnect() {
	// As long as we've got requests to preconnect, preconnect
	for {
		select {
		case <-p.closeCh:
			return
		case <-p.preconnects:
			conn, err := p.dialServer()
			if err != nil {
				log.Errorf("Unable to dial server %v: %s", p.Label(), err)
				p.MarkFailure()
				time.Sleep(250 * time.Millisecond)
			} else {
				// Failing to preconnect does indicate a failing proxy, but
				// considering multiplexed transports, successful preconnects
				// don't necessarily mean the proxy is good. Don't mark
				// success here.
				p.preconnected <- p.newPreconnected(conn)
			}
		}
	}
}

func (p *proxy) newPreconnected(conn serverConn) *proxyConnection {
	return &proxyConnection{
		proxy:     p,
		conn:      conn,
		expiresAt: time.Now().Add(expireTimeout),
	}
}

func (p *proxy) Preconnect() {
	select {
	case p.preconnects <- nil:
		// okay
	default:
		// maxPreconnects already requested, ignore
	}
}

func (p *proxy) NumPreconnecting() int {
	return len(p.preconnects)
}

func (p *proxy) NumPreconnected() int {
	return len(p.preconnected)
}

func (p *proxy) Preconnected() balancer.ProxyConnection {
	// always preconnect to replace the returned connection, or request a new
	// connection in case we didn't have any.
	defer p.Preconnect()

	for {
		select {
		case pc := <-p.preconnected:
			// found a preconnected conn
			if pc.expiresAt.Before(time.Now()) {
				// discard expired connection
				pc.conn.Close()
				continue
			}
			return pc
		default:
			// none immediately available and return nil
			return nil
		}
	}
}

// DialContext dials using provided context
func (pc *proxyConnection) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	upstream := false
	conn, err := pc.doDial(ctx, network, addr)
	if err != nil {
		if err == errUpstream {
			upstream = true
		} else {
			pc.MarkFailure()
		}
	} else if network == connect {
		// only mark success if we did a CONNECT request because that involves a
		// full round-trip to/from the proxy
		pc.markSuccess()
	}
	return conn, upstream, err
}

func (p *proxy) markSuccess() {
	p.emaSuccessRate.Update(1)
	atomic.AddInt64(&p.attempts, 1)
	atomic.AddInt64(&p.successes, 1)
	newCS := atomic.AddInt64(&p.consecSuccesses, 1)
	log.Debugf("Dialer %s consecutive successes: %d -> %d", p.Label(), newCS-1, newCS)
	// only when state is changing
	if newCS <= 2 {
		atomic.StoreInt64(&p.consecFailures, 0)
	}
}

func (p *proxy) MarkFailure() {
	p.emaSuccessRate.Update(0)
	atomic.AddInt64(&p.attempts, 1)
	atomic.AddInt64(&p.failures, 1)
	atomic.StoreInt64(&p.consecSuccesses, 0)
	newCF := atomic.AddInt64(&p.consecFailures, 1)
	log.Debugf("Dialer %s consecutive failures: %d -> %d", p.Label(), newCF-1, newCF)
	return
}

func (pc *proxyConnection) doDial(ctx context.Context, network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error

	op := ops.Begin("dial_for_balancer").ChainedProxy(pc.Name(), pc.Addr(), pc.Protocol(), pc.Network()).Set("dial_type", network)
	defer op.End()

	conn, err = pc.dialInternal(op, ctx, network, addr)
	if err != nil {
		return nil, op.FailIf(err)
	}
	conn = idletiming.Conn(conn, IdleTimeout, func() {
		log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), IdleTimeout)
	})
	return conn, nil
}

func (pc *proxyConnection) dialInternal(op *ops.Op, ctx context.Context, network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error
	chDone := make(chan bool)
	start := time.Now()
	go func() {
		conn, err = pc.conn.dialOrigin(ctx, network, addr)
		if err != nil {
			op.Set("idled", idletiming.IsIdled(conn))
		}
		select {
		case chDone <- true:
		default:
			if err == nil {
				log.Debugf("Connection to %s established too late, closing", addr)
				conn.Close()
			}
		}
	}()
	select {
	case <-chDone:
		return pc.withRateTracking(conn, addr), err
	case <-ctx.Done():
		return nil, errors.New("fail to dial origin after %+v", time.Since(start))
	}
}

// dialOrigin implements the method from serverConn. With standard proxies, this
// involves sending either a CONNECT request or a GET request to initiate a
// persistent connection to the upstream proxy.
func (conn defaultServerConn) dialOrigin(ctx context.Context, network, addr string) (net.Conn, error) {
	if deadline, set := ctx.Deadline(); set {
		conn.SetDeadline(deadline)
	}
	var err error
	// Look for our special hacked "connect" transport used to signal
	// that we should send a CONNECT request and tunnel all traffic through
	// that.
	switch network {
	case connect:
		log.Debugf("Sending CONNECT request")
		err = conn.p.sendCONNECT(addr, conn)
	case persistent:
		log.Debugf("Sending GET request to establish persistent HTTP connection")
		err = conn.p.initPersistentConnection(addr, conn)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	// Unset the deadline to avoid affecting later read/write on the connection.
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func (p *proxy) onRequest(req *http.Request) {
	p.AdaptRequest(req)
	common.AddCommonHeaders(p.user, req)
	// Request BBR metrics
	req.Header.Set("X-BBR", "y")
}

func (p *proxy) onFinish(op *ops.Op) {
	op.ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network())
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
		return errors.New("Error reading CONNECT response: %s", err)
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
	rtt := time.Since(reqTime)
	timing := resp.Header.Get(servertiming.HeaderKey)
	if timing != "" {
		header, err := servertiming.ParseHeader(timing)
		if err != nil {
			log.Errorf("Fail to parse Server-Timing header in CONNECT response: %v", err)
		} else {
			// dialupstream is the only metric for now, but more may be added later.
			for _, metric := range header.Metrics {
				if metric.Name == gp.MetricDialUpstream {
					p.updateEstRTT(rtt - metric.Duration)
				}
			}
		}
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

// serverConn represents a connection to a proxy server.
type serverConn interface {
	// dialOrigin dials out to the given origin address using the connected server
	dialOrigin(ctx context.Context, network, addr string) (net.Conn, error)

	Close() error
}

// defaultServerConn is the standard implementation of serverConn to typical
// lantern proxies.
type defaultServerConn struct {
	net.Conn
	p *proxy
}

func (p *proxy) defaultServerConn(conn net.Conn, err error) (serverConn, error) {
	if err != nil {
		return nil, err
	}
	return &defaultServerConn{conn, p}, err
}

// lazyServerConn is a serverConn that defers opening a connection until
// dialOrigin is called. This is useful for multiplexed protocols like
// lampshade where there's no cost to creating new connections and it's safer to
// do so as late as possible in case the underlying TCP connection has been
// closed since the serverConn was eagerly established.
type lazyServerConn func(ctx context.Context) (serverConn, error)

func (lsc lazyServerConn) dialOrigin(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := lsc(ctx)
	if err != nil {
		return nil, err
	}
	return conn.dialOrigin(ctx, network, addr)
}

func (lsc lazyServerConn) Close() error {
	return nil
}

// enhttpServerConn represents a serverConn to domain-fronted servers. Unlike a
// defaultServerConn, an enhttpServerConn doesn't actually have a real TCP/UDP
// connection, since it operates by making multiple HTTP requests via a CDN like
// CloudFront.
type enhttpServerConn struct {
	dial func(string, string) (net.Conn, error)
}

func (conn *enhttpServerConn) dialOrigin(ctx context.Context, network, addr string) (net.Conn, error) {
	dfConn, err := conn.dial(network, addr)
	if err == nil {
		dfConn = idletiming.Conn(dfConn, IdleTimeout, func() {
			log.Debug("enhttp connection idled")
		})
	}
	return dfConn, err
}

func (conn *enhttpServerConn) Close() error {
	return nil
}
