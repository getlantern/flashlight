package chained

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	config "github.com/getlantern/common"
	"github.com/getlantern/ema"
	"github.com/getlantern/enhttp"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

const (
	trustedSuffix = " (t)"

	// Below two values are based on suggestions in rfc6298
	rttAlpha    = 0.125
	rttDevAlpha = 0.25

	rttDevK          = 2   // Estimated RTT = mean RTT + 2 * deviation
	successRateAlpha = 0.7 // See example_ema_success_rate_test.go

	defaultMultiplexedPhysicalConns = 1
)

// InsecureSkipVerifyTLSMasqOrigin controls whether the origin certificate is verified when dialing
// a tlsmasq proxy. This can be used when testing against origins with self-signed certificates.
// This should be false in production as allowing a 3rd party to impersonate the origin could allow
// for a kind of probe.
var InsecureSkipVerifyTLSMasqOrigin = false

var (
	chainedDialTimeout          = 1 * time.Minute
	theForceAddr, theForceToken string

	tlsKeyLogWriter        io.Writer
	createKeyLogWriterOnce sync.Once
)

// proxyImpl is the interface to hide the details of client side logic for
// different types of pluggable transports.
type proxyImpl interface {
	// dialServer is to establish connection to the proxy server to the point
	// of being able to transfer application data.
	dialServer(op *ops.Op, ctx context.Context) (net.Conn, error)
	// close releases the resources associated with the implementation, if any.
	close()
}

// nopCloser is a mixin to implement a do-nothing close() method of proxyImpl.
type nopCloser struct{}

func (c nopCloser) close() {}

// CreateDialer creates a Proxy (balancer.Dialer) with supplied server info.
func CreateDialer(name string, s *ChainedServerInfo, uc common.UserConfig) (balancer.Dialer, error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	if s.Addr == "" {
		return nil, errors.New("Empty addr")
	}
	addr := s.Addr
	if s.MultiplexedAddr != "" {
		addr = s.MultiplexedAddr
	}
	transport := s.PluggableTransport
	switch transport {
	case "":
		transport = "http"
	case "http", "https", "utphttp", "utphttps":
		transport = strings.TrimRight(transport, "s")
		if s.Cert == "" {
		} else if len(s.KCPSettings) > 0 {
			transport = "kcp"
		} else {
			transport = transport + "s"
		}
	}
	network := "tcp"
	switch transport {
	case "utphttp", "utphttps", "utpobfs4", "quic", "quic_ietf", "oquic":
		network = "udp"
	}
	p, err := newProxy(name, addr, transport, network, s, uc)

	impl, err := createImpl(name, addr, transport, s, uc, p.reportDialCore)
	if err != nil {
		return nil, err
	}
	p.impl = impl

	if s.MultiplexedAddr != "" || transport == "utphttp" ||
		transport == "utphttps" || transport == "utpobfs4" ||
		transport == "tlsmasq" {
		p.impl, err = multiplexed(p.impl, name, s)
		if err != nil {
			return nil, err
		}
	}

	allowPreconnecting := false
	switch transport {
	case "http", "https", "utphttp", "utphttps", "obfs4", "utpobfs4", "tlsmasq":
		allowPreconnecting = true
	}
	if allowPreconnecting && s.MaxPreconnect > 0 {
		log.Debugf("Enabling preconnecting for %v", p.Label())
		// give ourselves a large margin for making sure we're not using idled preconnected connections
		expiration := IdleTimeout / 2
		p.impl = newPreconnectingDialer(name, s.MaxPreconnect, expiration, p.impl)
	}
	return p, err
}

func createImpl(name, addr, transport string, s *ChainedServerInfo, uc common.UserConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	coreDialer := func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error) {
		return reportDialCore(op, func() (net.Conn, error) {
			return netx.DialContext(ctx, "tcp", addr)
		})
	}
	if strings.HasPrefix(transport, "utp") {
		dialer, err := utpDialer()
		if err != nil {
			return nil, err
		}
		coreDialer = func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error) {
			return reportDialCore(op, func() (net.Conn, error) {
				return dialer(ctx, addr)
			})
		}
	}
	var impl proxyImpl
	var err error
	switch transport {
	case "", "http", "https", "utphttp", "utphttps":
		if s.Cert == "" {
			log.Errorf("No Cert configured for %s, will dial with plain tcp", addr)
			impl = newHTTPImpl(addr, coreDialer)
		} else if len(s.KCPSettings) > 0 {
			log.Errorf("KCP configured for %s, not using tls", addr)
			impl, err = newKCPImpl(s, reportDialCore)
		} else {
			log.Tracef("Cert configured for %s, will dial with tls", addr)
			impl, err = newHTTPSImpl(name, addr, s, uc, coreDialer)
		}
	case "obfs4", "utpobfs4":
		impl, err = newOBFS4Impl(name, addr, s, coreDialer)
	case "lampshade":
		impl, err = newLampshadeImpl(name, addr, s, reportDialCore)
	case "quic":
		impl, err = newQUIC0Impl(name, addr, s, reportDialCore)
	case "quic_ietf", "oquic":
		impl, err = newQUICImpl(name, addr, s, reportDialCore)
	case "wss":
		impl, err = newWSSImpl(addr, s, reportDialCore)
	case "tlsmasq":
		impl, err = newTLSMasqImpl(name, addr, s, uc, reportDialCore)
	default:
		err = errors.New("Unknown transport: %v", transport).With("addr", addr).With("plugabble-transport", transport)
	}
	if err != nil {
		return nil, err
	}

	return impl, err
}

// ForceProxy forces everything through the HTTP proxy at forceAddr using
// forceToken.
func ForceProxy(forceAddr string, forceToken string) {
	log.Debugf("Forcing proxying through proxy at %v using token %v", forceAddr, forceToken)
	theForceAddr, theForceToken = forceAddr, forceToken
}

func forceProxy(s *ChainedServerInfo) {
	s.Addr = theForceAddr
	s.AuthToken = theForceToken
	s.Cert = ""
	s.PluggableTransport = ""
}

// consecCounter is a counter that can extend on both directions. Its default
// value is zero. Inc() sets it to 1 or adds it by 1; Dec() sets it to -1 or
// minus it by 1. When called concurrently, it may have an incorrect absolute
// value, but always have the correct sign.
type consecCounter struct {
	v int64
}

func (c *consecCounter) Inc() {
	if v := atomic.LoadInt64(&c.v); v <= 0 {
		atomic.StoreInt64(&c.v, 1)
	} else {
		atomic.StoreInt64(&c.v, v+1)
	}
}

func (c *consecCounter) Dec() {
	if v := atomic.LoadInt64(&c.v); v >= 0 {
		atomic.StoreInt64(&c.v, -1)
	} else {
		atomic.StoreInt64(&c.v, v-1)
	}
}

func (c *consecCounter) Get() int64 {
	return atomic.LoadInt64(&c.v)
}

type coreDialer func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error)

type reportDialCoreFn func(op *ops.Op, dialCore func() (net.Conn, error)) (net.Conn, error)
type dialOriginFn func(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error)

type proxy struct {
	// Store int64's up front to ensure alignment of 64 bit words
	// See https://golang.org/pkg/sync/atomic/#pkg-note-BUG
	attempts            int64
	successes           int64
	consecSuccesses     int64
	failures            int64
	consecFailures      int64
	abe                 int64 // Mbps scaled by 1000
	probeSuccesses      uint64
	probeSuccessKBs     uint64
	probeFailures       uint64
	probeFailedKBs      uint64
	dataSent            uint64
	dataRecv            uint64
	consecReadSuccesses consecCounter
	name                string
	protocol            string
	network             string
	multiplexed         bool
	addr                string
	authToken           string
	location            config.ServerLocation
	user                common.UserConfig
	trusted             bool
	bias                int
	impl                proxyImpl
	dialOrigin          dialOriginFn
	emaRTT              *ema.EMA
	emaRTTDev           *ema.EMA
	emaSuccessRate      *ema.EMA
	mostRecentABETime   time.Time
	numPreconnecting    func() int
	numPreconnected     func() int
	closeCh             chan bool
	closeOnce           sync.Once
	mx                  sync.Mutex
}

func newProxy(name, addr, protocol, network string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	p := &proxy{
		name:             name,
		protocol:         protocol,
		network:          network,
		multiplexed:      s.MultiplexedAddr != "",
		addr:             addr,
		location:         s.Location,
		authToken:        s.AuthToken,
		user:             uc,
		trusted:          s.Trusted,
		bias:             s.Bias,
		dialOrigin:       defaultDialOrigin,
		emaRTT:           ema.NewDuration(0, rttAlpha),
		emaRTTDev:        ema.NewDuration(0, rttDevAlpha),
		emaSuccessRate:   ema.New(1, successRateAlpha), // Consider a proxy success when initializing
		numPreconnecting: func() int { return 0 },
		numPreconnected:  func() int { return 0 },
		closeCh:          make(chan bool, 1),
		consecSuccesses:  1, // be optimistic
	}

	if p.bias == 0 && s.ENHTTPURL != "" {
		// By default, do not prefer ENHTTP proxies. Use a very low bias as domain-
		// fronting is our very-last resort.
		p.bias = -10
	} else if len(s.KCPSettings) > 0 && p.bias == 0 {
		// KCP consumes a lot of bandwidth, so we want to bias against using it
		// unless everything else is blocked. However, we prefer it to
		// domain-fronting. We only default the bias if none was configured.
		p.bias = -1
	}

	if s.ENHTTPURL != "" {
		tr := &frontedTransport{rt: eventual.NewValue()}
		go func() {
			rt, ok := fronted.NewDirect(5 * time.Minute)
			if !ok {
				log.Errorf("Unable to initialize domain-fronting for enhttp")
				return
			}
			tr.rt.Set(rt)
		}()
		dial := enhttp.NewDialer(&http.Client{
			Transport: tr,
		}, s.ENHTTPURL)
		p.dialOrigin = func(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error) {
			dfConn, err := p.reportedDial(func(op *ops.Op) (net.Conn, error) { return dial(network, addr) })
			dfConn, err = overheadWrapper(true)(dfConn, op.FailIf(err))
			if err == nil {
				dfConn = idletiming.Conn(dfConn, IdleTimeout, func() {
					log.Debug("enhttp connection idled")
				})
			}
			return dfConn, err
		}
	}
	return p, nil
}

func (p *proxy) Protocol() string {
	return p.protocol
}

func (p *proxy) Network() string {
	return p.network
}

func (p *proxy) Addr() string {
	return p.addr
}

func (p *proxy) Name() string {
	return p.name
}

func (p *proxy) Label() string {
	return fmt.Sprintf("%v (%v)", p.name, p.addr)
}

func (p *proxy) JustifiedLabel() string {
	label := fmt.Sprintf("%-38v at %21v", p.name, p.addr)
	if p.trusted {
		label = label + trustedSuffix
	}
	return label
}

func (p *proxy) Location() (string, string, string) {
	return p.location.CountryCode, p.location.Country, p.location.City
}

func (p *proxy) Trusted() bool {
	return p.trusted
}

func (p *proxy) AdaptRequest(req *http.Request) {
	req.Header.Add(common.TokenHeader, p.authToken)
}

// update both RTT and its deviation per rfc6298
func (p *proxy) updateEstRTT(rtt time.Duration) {
	deviation := rtt - p.emaRTT.GetDuration()
	if deviation < 0 {
		deviation = -deviation
	}
	p.emaRTT.UpdateDuration(rtt)
	p.emaRTTDev.UpdateDuration(deviation)
}

// EstRTT implements the method from the balancer.Dialer interface. The
// value is updated from the round trip time of CONNECT request (minus the time
// to dial origin) or the HTTP ping. RTT deviation is also taken into account,
// so the value is higher if the proxy has a larger deviation over time, even if
// the measured RTT are the same.
func (p *proxy) EstRTT() time.Duration {
	if p.bias != 0 {
		// For biased proxies, return an extreme RTT in proportion to the bias
		return time.Duration(p.bias) * -100 * time.Second
	}
	return p.realEstRTT()
}

// realEstRTT() returns the same as EstRTT() but ignores any bias factor.
func (p *proxy) realEstRTT() time.Duration {
	// Take deviation into account, see rfc6298
	return time.Duration(p.emaRTT.Get() + rttDevK*p.emaRTTDev.Get())
}

// EstBandwidth implements the method from the balancer.Dialer interface.
//
// Bandwidth estimates are provided to clients following the below protocol:
//
// 1. On every inbound connection, we interrogate BBR congestion control
//    parameters to determine the estimated bandwidth, extrapolate this to what
//    we would expected for a 2.5 MB transfer using a linear estimation based on
//    how much data has actually been transferred on the connection and then
//    maintain an exponential moving average (EMA) of these estimates per remote
//    (client) IP.
// 2. If a client includes HTTP header "X-BBR: <anything>", we include header
//    X-BBR-ABE: <EMA bandwidth in Mbps> in the HTTP response.
// 3. If a client includes HTTP header "X-BBR: clear", we clear stored estimate
//    data for the client's IP.
func (p *proxy) EstBandwidth() float64 {
	if p.bias != 0 {
		// For biased proxies, return an extreme bandwidth in proportion to the bias
		return float64(p.bias) * 1000
	}
	return float64(atomic.LoadInt64(&p.abe)) / 1000
}

func (p *proxy) EstSuccessRate() float64 {
	return p.emaSuccessRate.Get()
}

func (p *proxy) setStats(attempts int64, successes int64, consecSuccesses int64, failures int64, consecFailures int64, emaRTT time.Duration, mostRecentABETime time.Time, abe int64, emaSuccessRate float64) {
	p.mx.Lock()
	atomic.StoreInt64(&p.attempts, attempts)
	atomic.StoreInt64(&p.successes, successes)
	atomic.StoreInt64(&p.consecSuccesses, consecSuccesses)
	atomic.StoreInt64(&p.failures, failures)
	atomic.StoreInt64(&p.consecFailures, consecFailures)
	p.emaRTT.SetDuration(emaRTT)
	p.mostRecentABETime = mostRecentABETime
	atomic.StoreInt64(&p.abe, abe)
	p.emaSuccessRate.Set(emaSuccessRate)
	p.mx.Unlock()
}

func (p *proxy) collectBBRInfo(reqTime time.Time, resp *http.Response) {
	_abe := resp.Header.Get("X-Bbr-Abe")
	if _abe != "" {
		resp.Header.Del("X-Bbr-Abe")
		abe, err := strconv.ParseFloat(_abe, 64)
		if err == nil {
			// Only update ABE if the request was more recent than that for the prior
			// value.
			p.mx.Lock()
			if reqTime.After(p.mostRecentABETime) {
				log.Debugf("%v: X-BBR-ABE: %.2f Mbps", p.Label(), abe)
				intABE := int64(abe * 1000)
				if intABE > 0 {
					// We check for a positive ABE here because in some scenarios (like
					// server restart) we can get 0 ABEs. In that case, we want to just
					// stick with whatever we've got so far.
					atomic.StoreInt64(&p.abe, intABE)
					p.mostRecentABETime = reqTime
				}
			}
			p.mx.Unlock()
		}
	}
}

func (p *proxy) reportDialCore(op *ops.Op, dialCore func() (net.Conn, error)) (net.Conn, error) {
	estRTT, estBandwidth := p.EstRTT(), p.EstBandwidth()
	if estRTT > 0 {
		op.SetMetricAvg("est_rtt_ms", estRTT.Seconds()*1000)
	}
	if estBandwidth > 0 {
		op.SetMetricAvg("est_mbps", estBandwidth)
	}
	elapsed := mtime.Stopwatch()
	conn, err := dialCore()
	delta := elapsed()
	log.Tracef("Core dial time to %v was %v", p.name, delta)
	op.CoreDialTime(delta, err)
	return overheadWrapper(false)(conn, err)
}

func (p *proxy) reportedDial(dial func(op *ops.Op) (net.Conn, error)) (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(p.name, p.addr, p.protocol, p.network, p.multiplexed)
	defer op.End()

	elapsed := mtime.Stopwatch()
	conn, err := dial(op)
	delta := elapsed()
	op.DialTime(delta, err)
	reportProxyDial(delta, err)

	return conn, op.FailIf(err)
}

// reportProxyDial reports a "proxy_dial" op if and only if the dial was
// successful or failed in a way that might indicate blocking.
func reportProxyDial(delta time.Duration, err error) {
	success := err == nil
	potentialBlocking := false
	if err != nil {
		errText := err.Error()
		potentialBlocking =
			!strings.Contains(errText, "network is down") &&
				!strings.Contains(errText, "unreachable") &&
				!strings.Contains(errText, "Bad status code on CONNECT response")
	}
	if success || potentialBlocking {
		innerOp := ops.Begin("proxy_dial")
		innerOp.DialTime(delta, err)
		innerOp.FailIf(err)
		innerOp.End()
	}
}

func splitClientHello(hello []byte) [][]byte {
	const minSplits, maxSplits = 2, 5
	var (
		maxLen = len(hello) / minSplits
		splits = [][]byte{}
		start  = 0
		end    = start + rand.Intn(maxLen) + 1
	)
	for end < len(hello) && len(splits) < maxSplits-1 {
		splits = append(splits, hello[start:end])
		start = end
		end = start + rand.Intn(maxLen) + 1
	}
	splits = append(splits, hello[start:])
	return splits
}
