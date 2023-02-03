package chained

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	servertiming "github.com/mitchellh/go-server-timing"

	"github.com/getlantern/bufconn"
	"github.com/getlantern/errors"
	"github.com/getlantern/idletiming"
	gp "github.com/getlantern/proxy/v2"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/bandwidth"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

const (
	minCheckInterval      = 10 * time.Second
	maxCheckInterval      = 15 * time.Minute
	dialCoreCheckInterval = 30 * time.Second
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

func (p *proxy) Stop() {
	log.Tracef("Stopping dialer %s", p.Label())
	p.closeOnce.Do(func() {
		close(p.closeCh)
		p.impl.Close()
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
	return p.ConsecFailures() == 0 ||
		(p.EstSuccessRate() > 0.5 && p.consecReadSuccesses.Get() > 0)
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

func (p *proxy) WriteStats(w io.Writer) {
	estRTT := p.EstRTT().Seconds()
	estBandwidth := p.EstBandwidth()
	probeSuccesses, probeSuccessKBs, probeFailures, probeFailedKBs := p.ProbeStats()
	_, _ = fmt.Fprintf(w, "%s  P:%3d  R:%3d  A: %5d  S: %5d  CS: %3d  F: %5d  CF: %3d  R: %4.3f  L: %4.0fms  B: %6.2fMbps  T: %7s/%7s  P: %3d(%3dkb)/%3d(%3dkb)\n",
		p.JustifiedLabel(),
		p.NumPreconnected(),
		p.NumPreconnecting(),
		p.Attempts(),
		p.Successes(), p.ConsecSuccesses(),
		p.Failures(), p.ConsecFailures(),
		p.EstSuccessRate(),
		estRTT*1000, estBandwidth,
		humanize.Bytes(p.DataSent()), humanize.Bytes(p.DataRecv()),
		probeSuccesses, probeSuccessKBs, probeFailures, probeFailedKBs)
	if impl, ok := p.impl.(*multipathImpl); ok {
		for _, line := range impl.FormatStats() {
			_, _ = fmt.Fprintf(w, "\t%s\n", line)
		}
	}
}

// DialContext dials using provided context
func (p *proxy) DialContext(ctx context.Context, network, addr string) (conn net.Conn, isUpstreamError bool, err error) {
	op := ops.Begin("dial_for_balancer").
		ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network(), p.multiplexed).
		Set("dial_type", network)
	defer op.End()

	log.Debugf("Dialing origin address %s for proxy %s", addr, p.Label())
	conn, err = p.dialOrigin(op, ctx, p, network, addr)
	if err != nil {
		op.Set("idled", idletiming.IsIdled(conn))
		op.FailIf(err)
		if err != errUpstream {
			p.MarkFailure()
		}
		return nil, err == errUpstream, err
	}

	conn = p.withRateTracking(conn, addr, ctx)
	if network == balancer.NetworkConnect {
		// only mark success if we did a CONNECT request because that involves a
		// full round-trip to/from the proxy
		p.markSuccess()
	}
	return conn, false, nil
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
	return
}

func (p *proxy) SuccessfulPrefixChan() <-chan []byte {
	if p.opts == nil || p.opts.successfulPrefixChan == nil {
		return nil
	}
	return p.opts.successfulPrefixChan
}

type ConnectionWithPrefix struct {
	conn      net.Conn
	prefixBuf []byte
	err       error
}

func defaultDialOrigin(
	op *ops.Op,
	ctx context.Context,
	p *proxy,
	network, addr string) (net.Conn, error) {
	// Drain the successfulPrefixChan if it exists. This is to prevent
	// the channel from filling up and blocking the proxy.
	if p.opts.successfulPrefixChan != nil {
	looper:
		select {
		case <-p.opts.successfulPrefixChan:
			goto looper
		default:
			break looper
		}
	}

	// Check how many prefixes we have for this proxy.
	if p.opts == nil || len(p.opts.Prefixes) == 0 {
		collectionCh := make(chan ConnectionWithPrefix, 1)
		go doDialOrigin(op, ctx, p, network, addr, nil, collectionCh)
		connectionCollection := <-collectionCh
		return connectionCollection.conn, connectionCollection.err
	}

	// We have prefixes, so we'll use multipath dialing.

	// Dial routine: dials the server len(p.Prefixes) times, each time with a
	// different prefix. The successful connection with the lowest RTT will be
	// used. The rest will still be reported as a success, but will be closed
	// immediately.
	collectionCh := make(chan ConnectionWithPrefix, len(p.opts.Prefixes))
	for _, prefixBuf := range p.opts.Prefixes {
		go doDialOrigin(op, ctx, p, network, addr, prefixBuf, collectionCh)
	}

	// Collection routine
	successfulConnCh := make(chan net.Conn)
	sb := &strings.Builder{}
	go func() {
		didSendSuccessfulConn := false
		for pc := range collectionCh {
			// If we get an error, we can't continue.
			// Collect the error so we can log it later if everything fails.
			if pc.err != nil {
				log.Debugf("Dialer %s failed to dial with prefix %s: %s", p.Label(), pc.prefixBuf, pc.err)
				sb.WriteString(fmt.Sprintf(" %s: %s", pc.prefixBuf, pc.err))
				continue
			}

			// Signal a correct dial with a prefix
			if p.opts.successfulPrefixChan != nil {
				p.opts.successfulPrefixChan <- pc.prefixBuf
			}

			// If this is the first successful dial, send it to the main routine
			// TODO <26-01-2023, soltzen> Consider using closed channels
			// instead of a boolean
			if !didSendSuccessfulConn {
				successfulConnCh <- pc.conn
				didSendSuccessfulConn = true
				continue
			}

			// Close the connection since if we're here, we already sent a
			// successful connection.
			pc.conn.Close()
		}
	}()

	// Wait for a successful connection or a timeout
	select {
	case conn := <-successfulConnCh:
		return conn, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("Failed to dial %s with any prefix: %s",
			p.Addr(), sb.String())
	}
}

// doDialOrigin implements the method from serverConn. With standard proxies, this
// involves sending either a CONNECT request or a GET request to initiate a
// persistent connection to the upstream proxy.
func doDialOrigin(
	op *ops.Op,
	ctx context.Context,
	p *proxy,
	network, addr string,
	prefixBuf []byte,
	collectionCh chan<- ConnectionWithPrefix) {
	conn, err := p.reportedDial(func(op *ops.Op) (net.Conn, error) {
		return p.impl.DialServer(op, ctx, prefixBuf)
	})
	if err != nil {
		log.Debugf("Unable to dial server %v: %s", p.Label(), err)
		collectionCh <- ConnectionWithPrefix{
			err: fmt.Errorf("dialing server: %s", err)}
		return
	}

	conn, err = overheadWrapper(true)(conn, op.FailIf(err))
	var timeout time.Duration
	if deadline, set := ctx.Deadline(); set {
		conn.SetDeadline(deadline)
		// Set timeout based on our given deadline, minus a 2 second fudge factor
		timeUntilDeadline := deadline.Sub(time.Now())
		timeout = timeUntilDeadline - 2*time.Second
		if timeout < 0 {
			log.Errorf("Not enough time left for server to dial upstream within %v, return errUpstream immediately", timeUntilDeadline)
			collectionCh <- ConnectionWithPrefix{err: errUpstream}
			return
		}
	}

	// Look for our special hacked "connect" transport used to signal
	// that we should send a CONNECT request and tunnel all traffic through
	// that.
	switch network {
	case balancer.NetworkConnect:
		log.Trace("Sending CONNECT request")
		bconn := bufconn.Wrap(conn)
		conn = bconn
		err = p.sendCONNECT(op, addr, bconn, timeout)
	case balancer.NetworkPersistent:
		log.Trace("Sending GET request to establish persistent HTTP connection")
		err = p.initPersistentConnection(addr, conn)
	}
	if err != nil {
		conn.Close()
		log.Debugf("Unable to dial server %v: %s", p.Label(), err)
		collectionCh <- ConnectionWithPrefix{err: fmt.Errorf("dialing server: %s", err)}
		return
	}
	// Unset the deadline to avoid affecting later read/write on the connection.
	conn.SetDeadline(time.Time{})
	collectionCh <- ConnectionWithPrefix{conn: conn, prefixBuf: prefixBuf}
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
	err = p.checkCONNECTResponse(op, r, req, reqTime)
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

func (p *proxy) checkCONNECTResponse(op *ops.Op, r *bufio.Reader, req *http.Request, reqTime time.Time) error {
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
