package chained

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"

	"github.com/getlantern/bufconn"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/bandwidth"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/netx"
	gp "github.com/getlantern/proxy"
)

var (
	// IdleTimeout closes connections idle for a period to avoid dangling
	// connections. Web applications tend to contact servers in 1 minute
	// interval or below. 65 seconds is long enough to avoid interrupt normal
	// connections but shorter than the idle timeout on the server to avoid
	// running into closed connection problems.
	IdleTimeout = 65 * time.Second

	// errUpstream is an error that indicates there was a problem upstream of a
	// proxy. Such errors are not counted as failures but do allow failover to
	// other proxies.
	errUpstream = errors.New("Upstream error")
)

type proxyDialer interface {
	dialServer(context.Context, *proxy) (net.Conn, error)
	dialOrigin(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error)
}

func (p *proxy) Stop() {
	log.Tracef("Stopping dialer %s", p.Label())
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
	return p.EstSuccessRate() > 0.5 &&
		p.consecReadSuccesses.Get() > 0
}

func (p *proxy) DataSent() uint64 {
	return atomic.LoadUint64(&p.dataSent)
}

func (p *proxy) DataRecv() uint64 {
	return atomic.LoadUint64(&p.dataRecv)
}

func (p *proxy) NumPreconnecting() int {
	return p.numPreconnecting()
}

func (p *proxy) NumPreconnected() int {
	return p.numPreconnected()
}

// DialContext dials using provided context
func (p *proxy) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	upstream := false
	conn, err := p.doDial(ctx, network, addr)
	if err != nil {
		if err == errUpstream {
			upstream = true
		} else {
			p.MarkFailure()
		}
	} else if network == balancer.NetworkConnect {
		// only mark success if we did a CONNECT request because that involves a
		// full round-trip to/from the proxy
		p.markSuccess()
	}
	return conn, upstream, err
}

func (p *proxy) markSuccess() {
	p.emaSuccessRate.Update(1)
	atomic.AddInt64(&p.attempts, 1)
	atomic.AddInt64(&p.successes, 1)
	newCS := atomic.AddInt64(&p.consecSuccesses, 1)
	log.Tracef("Dialer %s consecutive successes: %d -> %d", p.Label(), newCS-1, newCS)
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
	log.Tracef("Dialer %s consecutive failures: %d -> %d", p.Label(), newCF-1, newCF)
}

func (p *proxy) doDial(ctx context.Context, network, addr string) (net.Conn, error) {
	op := ops.Begin("dial_for_balancer").ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network(), p.multiplexed).Set("dial_type", network)
	defer op.End()

	conn, err := p.dialOrigin(op, ctx, p, network, addr)
	if err != nil {
		op.Set("idled", idletiming.IsIdled(conn))
		return nil, op.FailIf(err)
	}
	conn = idletiming.Conn(p.withRateTracking(conn, addr, ctx), IdleTimeout, func() {
		op := ops.BeginWithBeam("idle_close", ctx)
		log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), IdleTimeout)
		op.End()
	})
	return conn, nil
}

// defaultDialOrigin sends either a CONNECT request or a GET request to initiate a
// persistent connection to the upstream proxy.
func defaultDialOrigin(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error) {
	log.Debugf("Dialing origin to %v", addr)
	conn, err := p.dialServer(ctx)
	if err != nil {
		return nil, err
	}

	var timeout time.Duration
	if deadline, set := ctx.Deadline(); set {
		conn.SetDeadline(deadline)
		// Set timeout based on our given deadline, minus a 2 second fudge factor
		timeUntilDeadline := time.Until(deadline)
		timeout = timeUntilDeadline - 2*time.Second
		if timeout < 0 {
			log.Errorf("Not enough time left for server to dial upstream within %v, return errUpstream immediately", timeUntilDeadline)
			return nil, errUpstream
		}
	}
	// Look for our special hacked "connect" transport used to signal
	// that we should send a CONNECT request and tunnel all traffic through
	// that.
	switch network {
	case balancer.NetworkConnect:
		log.Debug("Sending CONNECT request")
		bconn := bufconn.Wrap(conn)
		conn = bconn
		err = p.sendCONNECT(op, addr, bconn, timeout)
	case balancer.NetworkPersistent:
		log.Debug("Sending GET request to establish persistent HTTP connection")
		err = p.initPersistentConnection(addr, conn)
	}
	if err != nil {
		log.Debugf("Error sending CONNECT or persistent...closing possibly virtual connection to: %v", addr)
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
	op.ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network(), p.multiplexed)
}

func (p *proxy) sendCONNECT(op *ops.Op, addr string, conn bufconn.Conn, timeout time.Duration) error {
	reqTime := time.Now()
	req, err := p.buildCONNECTRequest(addr, timeout)
	if err != nil {
		return fmt.Errorf("Unable to construct CONNECT request: %s", err)
	}
	err = req.Write(conn)
	if err != nil {
		return fmt.Errorf("Unable to write CONNECT request: %s", err)
	}

	r := conn.Head()
	err = p.checkCONNECTResponse(op, r, req, reqTime, conn)
	if err != nil {
		log.Errorf("Error checking CONNECT response on conn: %#v", conn.(netx.WrappedConn).Wrapped().(fmt.Stringer).String())
	}
	return err
}

func (p *proxy) buildCONNECTRequest(addr string, timeout time.Duration) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodConnect, "/", nil)
	if err != nil {
		return nil, err
	}
	req.URL = &url.URL{
		Host: addr,
	}
	req.Host = addr
	if timeout != 0 {
		req.Header.Set(common.ProxyDialTimeoutHeader, fmt.Sprint(int(math.Ceil(timeout.Seconds()*1000))))
	}
	p.onRequest(req)
	return req, nil
}

func (p *proxy) checkCONNECTResponse(op *ops.Op, r *bufio.Reader, req *http.Request, reqTime time.Time, conn bufconn.Conn) error {
	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return errors.New("Error reading CONNECT response after %v: %s on %v", time.Since(reqTime), err, conn)
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
					adjustedRTT := rtt - metric.Duration
					op.Set("connect_time_total", rtt)
					op.Set("connect_time_server", metric.Duration)
					op.Set("connect_time_rtt", adjustedRTT)
					p.updateEstRTT(adjustedRTT)
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
