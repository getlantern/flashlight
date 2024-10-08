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

	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/common/config"
	"github.com/getlantern/ema"
	"github.com/getlantern/errors"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"

	"github.com/getlantern/flashlight/v7/bandit"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/proxied"
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

type connWrapper func(net.Conn, *config.ProxyConfig) net.Conn

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

// CreateDialers creates a list of Proxies (bandit.Dialer) with supplied server info.
func CreateDialers(configDir string, proxies map[string]*config.ProxyConfig, uc common.UserConfig) []bandit.Dialer {
	return lo.Values(CreateDialersMap(configDir, proxies, uc))
}

// CreateDialersMap creates a map of Proxies (bandit.Dialer) with supplied server info.
func CreateDialersMap(configDir string, proxies map[string]*config.ProxyConfig, uc common.UserConfig) map[string]bandit.Dialer {
	groups := groupByMultipathEndpoint(proxies)

	// We parallelize the creation of the dialers because some of them may take
	// a long time to initialize (e.g. tls-based proxies that try to read
	// browser hellos on creation time).
	wg := &sync.WaitGroup{}
	var m sync.Map

	for endpoint, group := range groups {
		if endpoint == "" {
			log.Debugf("Creating map for %d individual chained servers", len(group))
			for name, s := range group {
				wg.Add(1)
				go func(name string, s *config.ProxyConfig) {
					defer wg.Done()
					dialer, err := CreateDialer(configDir, name, s, uc)
					if err != nil {
						log.Errorf("Unable to configure chained server %v. Received error: %v", name, err)
						return
					}
					log.Debugf("Adding chained server: %v", dialer.JustifiedLabel())
					m.Store(name, dialer)
				}(name, s)
			}
		} else {
			log.Debugf("Creating map for %d chained servers for multipath endpoint %s", len(group), endpoint)
			wg.Add(1)
			go func(endpoint string, group map[string]*config.ProxyConfig) {
				defer wg.Done()
				dialer, err := CreateMPDialer(configDir, endpoint, group, uc)
				if err != nil {
					log.Errorf("Unable to configure multipath server to %v. Received error: %v", endpoint, err)
					return
				}
				m.Store(endpoint, dialer)
			}(endpoint, group)
		}
	}
	wg.Wait()
	mappedDialers := make(map[string]bandit.Dialer)
	m.Range(func(k, v interface{}) bool {
		mappedDialers[k.(string)] = v.(bandit.Dialer)
		return true
	})

	return mappedDialers
}

// CreateDialer creates a Proxy (balancer.Dialer) with supplied server info.
func CreateDialer(configDir, name string, s *config.ProxyConfig, uc common.UserConfig) (bandit.Dialer, error) {
	addr, transport, network, err := extractParams(s)
	if err != nil {
		return nil, err
	}
	p, err := newProxy(name, addr, transport, network, s, uc)
	if err != nil {
		return nil, err
	}
	log.Debugf("AuthToken: %s", p.authToken)
	p.impl, err = createImpl(configDir, name, addr, transport, s, uc, p.reportDialCore)
	if err != nil {
		log.Debugf("Unable to create proxy implementation for %v: %v", name, err)
		return nil, err
	}
	return p, nil
}

func extractParams(s *config.ProxyConfig) (addr, transport, network string, err error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	if s.Addr == "" {
		err = errors.New("Empty addr")
		return
	}
	addr = s.Addr
	if s.MultiplexedAddr != "" {
		addr = s.MultiplexedAddr
	}
	transport = s.PluggableTransport
	switch transport {
	case "":
		transport = "http"
	case "http", "https", "utphttp", "utphttps":
		transport = strings.TrimRight(transport, "s")
		if s.Cert == "" {
		} else {
			transport = transport + "s"
		}
	}
	network = "tcp"
	switch transport {
	case "quic_ietf":
		network = "udp"
	}
	return
}

func createImpl(configDir, name, addr, transport string, s *config.ProxyConfig, uc common.UserConfig,
	reportDialCore reportDialCoreFn) (proxyImpl, error) {
	coreDialer := func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error) {
		return reportDialCore(op, func() (net.Conn, error) {
			return netx.DialContext(ctx, "tcp", addr)
		})
	}
	var impl proxyImpl
	var err error
	switch transport {
	case "", "http", "https":
		if s.Cert == "" {
			log.Errorf("No Cert configured for %s, will dial with plain tcp", addr)
			impl = newHTTPImpl(addr, coreDialer)
		} else {
			log.Tracef("Cert configured for %s, will dial with tls", addr)
			impl, err = newHTTPSImpl(configDir, name, addr, s, uc, coreDialer,
				wrapTLSFrag)
		}
	case "quic_ietf":
		impl, err = newQUICImpl(name, addr, s, reportDialCore)
	case "shadowsocks":
		impl, err = newShadowsocksImpl(name, addr, s, reportDialCore)
	case "wss":
		impl, err = newWSSImpl(addr, s, reportDialCore)
	case "tlsmasq":
		impl, err = newTLSMasqImpl(configDir, name, addr, s, uc, reportDialCore,
			wrapTLSFrag)
	case "starbridge":
		impl, err = newStarbridgeImpl(name, addr, s, reportDialCore)
	case "broflake":
		impl, err = newBroflakeImpl(s, reportDialCore)
	case "algeneva":
		impl, err = newAlgenevaImpl(addr, s, reportDialCore)
	case "water":
		impl, err = newWaterImpl(addr, s, reportDialCore, &http.Client{
			Transport: proxied.ParallelForIdempotent(),
			Timeout:   30 * time.Second,
		})
	default:
		err = errors.New("Unknown transport: %v", transport).With("addr", addr).With("plugabble-transport", transport)
	}
	if err != nil {
		return nil, err
	}

	allowPreconnecting := false
	switch transport {
	case "https", "tlsmasq":
		allowPreconnecting = true
	}

	if s.MultiplexedAddr != "" || isAMultiplexedTransport(transport) {
		impl, err = multiplexed(impl, name, s)
		if err != nil {
			return nil, err
		}
	} else if allowPreconnecting && s.MaxPreconnect > 0 {
		log.Debugf("Enabling preconnecting for %v", name)
		// give ourselves a large margin for making sure we're not using idled preconnected connections
		expiration := IdleTimeout / 2
		impl = newPreconnectingDialer(name, int(s.MaxPreconnect), expiration, impl)
	}

	return impl, err
}

func isAMultiplexedTransport(transport string) bool {
	return transport == "tlsmasq" ||
		transport == "starbridge" ||
		transport == "algeneva"
}

// ForceProxy forces everything through the HTTP proxy at forceAddr using
// forceToken.
func ForceProxy(forceAddr string, forceToken string) {
	log.Debugf("Forcing proxying through proxy at %v using token %v", forceAddr, forceToken)
	theForceAddr, theForceToken = forceAddr, forceToken
}

func forceProxy(s *config.ProxyConfig) {
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

type proxy struct {
	// Store int64's up front to ensure alignment of 64 bit words
	// See https://golang.org/pkg/sync/atomic/#pkg-note-BUG
	attempts            int64
	successes           int64
	consecSuccesses     int64
	failures            int64
	consecFailures      int64
	abe                 int64 // Mbps scaled by 1000
	dataSent            uint64
	dataRecv            uint64
	consecReadSuccesses consecCounter
	name                string
	protocol            string
	network             string
	multiplexed         bool
	addr                string
	authToken           string
	location            *config.ProxyConfig_ProxyLocation
	user                common.UserConfig
	trusted             bool
	bias                int
	impl                proxyImpl
	emaRTT              *ema.EMA
	emaRTTDev           *ema.EMA
	emaSuccessRate      *ema.EMA
	mostRecentABETime   time.Time
	numPreconnecting    func() int
	numPreconnected     func() int
	allowedDomains      *domainrouting.Rules
	closeCh             chan bool
	closeOnce           sync.Once
	mx                  sync.Mutex
}

func newProxy(name, addr, protocol, network string, s *config.ProxyConfig, uc common.UserConfig) (*proxy, error) {
	p := &proxy{
		name:             name,
		protocol:         protocol,
		network:          network,
		multiplexed:      s.MultiplexedAddr != "",
		addr:             addr,
		authToken:        s.AuthToken,
		user:             uc,
		trusted:          s.Trusted,
		bias:             int(s.Bias),
		emaRTT:           ema.NewDuration(0, rttAlpha),
		emaRTTDev:        ema.NewDuration(0, rttDevAlpha),
		emaSuccessRate:   ema.New(1, successRateAlpha), // Consider a proxy success when initializing
		numPreconnecting: func() int { return 0 },
		numPreconnected:  func() int { return 0 },
		closeCh:          make(chan bool, 1),
		consecSuccesses:  1, // be optimistic
	}
	// Make sure we don't panic if there's no location.
	if s.Location != nil {
		p.location = proto.Clone(s.Location).(*config.ProxyConfig_ProxyLocation)
	}

	if len(s.AllowedDomains) > 0 {
		// Some proxies like Broflake only support a limited set of domains. This sets up domain routing
		// rules based on what was configured in the proxy config.
		rulesMap := make(domainrouting.RulesMap)
		for _, domain := range s.AllowedDomains {
			rulesMap[domain] = domainrouting.MustProxy
		}
		p.allowedDomains = domainrouting.NewRules(rulesMap)
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
	if p.location == nil {
		return "", "", ""
	}
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

// EstRTT implements the method from the bandit.Dialer interface. The
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

// EstBandwidth implements the method from the bandit.Dialer interface.
//
// Bandwidth estimates are provided to clients following the below protocol:
//
//  1. On every inbound connection, we interrogate BBR congestion control
//     parameters to determine the estimated bandwidth, extrapolate this to what
//     we would expected for a 2.5 MB transfer using a linear estimation based on
//     how much data has actually been transferred on the connection and then
//     maintain an exponential moving average (EMA) of these estimates per remote
//     (client) IP.
//  2. If a client includes HTTP header "X-BBR: <anything>", we include header
//     X-BBR-ABE: <EMA bandwidth in Mbps> in the HTTP response.
//  3. If a client includes HTTP header "X-BBR: clear", we clear stored estimate
//     data for the client's IP.
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

func (p *proxy) DialProxy(ctx context.Context) (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(p.name, p.addr, p.protocol, p.network, p.multiplexed)
	defer op.End()
	return p.impl.dialServer(op, ctx)
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
	return conn, err
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
				!strings.Contains(errText, "no route to host") &&
				!strings.Contains(errText, "unreachable") &&
				!strings.Contains(errText, "Bad status code on CONNECT response") &&
				!strings.Contains(errText, "Unable to protect") &&
				!strings.Contains(errText, "INTERNAL_ERROR") &&
				!strings.Contains(errText, "operation not permitted") &&
				!strings.Contains(errText, "forbidden")
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
