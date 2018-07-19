package chained

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"

	"github.com/getlantern/ema"
	"github.com/getlantern/enhttp"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/fronted"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/kcpwrapper"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lampshade"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsdialer"
	"github.com/mitchellh/mapstructure"
	"github.com/refraction-networking/utls"
	"github.com/tevino/abool"
)

const (
	trustedSuffix = " (t)"

	defaultInitPreconnect = 20
	defaultMaxPreconnect  = 100

	// Below two values are based on suggestions in rfc6298
	rttAlpha    = 0.125
	rttDevAlpha = 0.25

	rttDevK          = 2   // Estimated RTT = mean RTT + 2 * deviation
	SuccessRateAlpha = 0.7 // See example_ema_success_rate_test.go
)

var (
	chainedDialTimeout          = 1 * time.Minute
	theForceAddr, theForceToken string
)

// CreateDialer creates a Proxy (balancer.Dialer) with supplied server info.
func CreateDialer(name string, s *ChainedServerInfo, uc common.UserConfig) (balancer.Dialer, error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	if s.Addr == "" {
		return nil, errors.New("Empty addr")
	}
	switch s.PluggableTransport {
	case "":
		var p *proxy
		var err error
		if s.Cert == "" {
			log.Errorf("No Cert configured for %s, will dial with plain tcp", s.Addr)
			p, err = newHTTPProxy(name, s, uc)
		} else {
			log.Tracef("Cert configured for  %s, will dial with tls", s.Addr)
			p, err = newHTTPSProxy(name, s, uc)
		}
		return p, err
	case "obfs4":
		return newOBFS4Proxy(name, s, uc)
	case "lampshade":
		return newLampshadeProxy(name, s, uc)
	default:
		return nil, errors.New("Unknown transport: %v", s.PluggableTransport).With("addr", s.Addr).With("plugabble-transport", s.PluggableTransport)
	}
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

func newHTTPProxy(name string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	var doDialServer func(ctx context.Context, p *proxy) (serverConn, error)
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
		doDialServer = func(ctx context.Context, p *proxy) (serverConn, error) {
			return &enhttpServerConn{dial}, nil
		}
	} else {
		doDialServer = func(ctx context.Context, p *proxy) (serverConn, error) {
			return p.reportedDial(p.addr, p.protocol, p.network, func(op *ops.Op) (net.Conn, error) {
				return p.dialCore(op)(ctx)
			})
		}
	}

	return newProxy(name, "http", "tcp", s.Addr, s, uc, s.ENHTTPURL != "", doDialServer)
}

func newHTTPSProxy(name string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	x509cert := cert.X509()

	return newProxy(name, "https", "tcp", s.Addr, s, uc, s.Trusted, func(ctx context.Context, p *proxy) (serverConn, error) {
		return p.reportedDial(p.addr, p.protocol, p.network, func(op *ops.Op) (net.Conn, error) {
			tlsConfig, clientHelloID := tlsConfigForProxy(s)
			td := &tlsdialer.Dialer{
				DoDial: func(network, addr string, timeout time.Duration) (net.Conn, error) {
					return p.dialCore(op)(ctx)
				},
				Timeout:        timeoutFor(ctx),
				SendServerName: tlsConfig.ServerName != "",
				Config:         tlsConfig,
				ClientHelloID:  clientHelloID,
			}
			conn, err := td.Dial("tcp", p.addr)
			if err != nil {
				return conn, err
			}
			if !conn.ConnectionState().PeerCertificates[0].Equal(x509cert) {
				if closeErr := conn.Close(); closeErr != nil {
					log.Debugf("Error closing chained server connection: %s", closeErr)
				}
				var received interface{}
				var expected interface{}
				_received, certErr := keyman.LoadCertificateFromX509(conn.ConnectionState().PeerCertificates[0])
				if certErr != nil {
					log.Errorf("Unable to parse received certificate: %v", certErr)
					received = conn.ConnectionState().PeerCertificates[0]
					expected = x509cert
				} else {
					received = string(_received.PEMEncoded())
					expected = string(cert.PEMEncoded())
				}
				return nil, op.FailIf(log.Errorf("Server's certificate didn't match expected! Server had\n%v\nbut expected:\n%v",
					received, expected))
			}
			return overheadWrapper(true)(conn, op.FailIf(err))
		})
	})
}

func newOBFS4Proxy(name string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	if s.Cert == "" {
		return nil, fmt.Errorf("No Cert configured for obfs4 server, can't connect")
	}

	cf, err := (&obfs4.Transport{}).ClientFactory("")
	if err != nil {
		return nil, log.Errorf("Unable to create obfs4 client factory: %v", err)
	}

	ptArgs := &pt.Args{}
	ptArgs.Add("cert", s.Cert)
	ptArgs.Add("iat-mode", s.ptSetting("iat-mode"))

	args, err := cf.ParseArgs(ptArgs)
	if err != nil {
		return nil, log.Errorf("Unable to parse client args: %v", err)
	}

	return newProxy(name, "obfs4", "tcp", s.Addr, s, uc, s.Trusted, func(ctx context.Context, p *proxy) (serverConn, error) {
		return p.reportedDial(p.Addr(), p.Protocol(), p.Network(), func(op *ops.Op) (net.Conn, error) {
			dialFn := func(network, address string) (net.Conn, error) {
				// We know for sure the network and address are the same as what
				// the inner DailServer uses.
				return p.dialCore(op)(ctx)
			}

			// The proxy it wrapped already has timeout applied.
			return overheadWrapper(true)(cf.Dial("tcp", p.addr, dialFn, args))
		})
	})
}

func newLampshadeProxy(name string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	rsaPublicKey, ok := cert.X509().PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("Public key is not an RSA public key!")
	}
	cipherCode := lampshade.Cipher(s.ptSettingInt(fmt.Sprintf("cipher_%v", runtime.GOARCH)))
	if cipherCode == 0 {
		if runtime.GOARCH == "amd64" {
			// On 64-bit Intel, default to AES128_GCM which is hardware accelerated
			cipherCode = lampshade.AES128GCM
		} else {
			// default to ChaCha20Poly1305 which is fast even without hardware acceleration
			cipherCode = lampshade.ChaCha20Poly1305
		}
	}
	windowSize := s.ptSettingInt("windowsize")
	maxPadding := s.ptSettingInt("maxpadding")
	maxStreamsPerConn := uint16(s.ptSettingInt("streams"))
	idleInterval, parseErr := time.ParseDuration(s.ptSetting("idleinterval"))
	if parseErr != nil || idleInterval < 0 {
		idleInterval = IdleTimeout * 2
		log.Debugf("Defaulted lampshade idleinterval to %v", idleInterval)
	}
	pingInterval, parseErr := time.ParseDuration(s.ptSetting("pinginterval"))
	if parseErr != nil || pingInterval < 0 {
		pingInterval = 15 * time.Second
		log.Debugf("Defaulted lampshade pinginterval to %v", pingInterval)
	}
	dialer := lampshade.NewDialer(&lampshade.DialerOpts{
		WindowSize:        windowSize,
		MaxPadding:        maxPadding,
		MaxStreamsPerConn: maxStreamsPerConn,
		IdleInterval:      idleInterval,
		PingInterval:      pingInterval,
		Pool:              buffers.Pool,
		Cipher:            cipherCode,
		ServerPublicKey:   rsaPublicKey,
	})
	dial := func(unusedCtx context.Context, p *proxy) (serverConn, error) {
		return lazyServerConn(func(ctx context.Context) (serverConn, error) {
			return p.reportedDial(s.Addr, "lampshade", "tcp", func(op *ops.Op) (net.Conn, error) {
				op.Set("ls_win", windowSize).
					Set("ls_pad", maxPadding).
					Set("ls_streams", int(maxStreamsPerConn)).
					Set("ls_cipher", cipherCode.String())
				conn, err := dialer.Dial(func() (net.Conn, error) {
					conn, err := p.dialCore(op)(ctx)
					if err == nil && idleInterval > 0 {
						conn = idletiming.Conn(conn, idleInterval, func() {
							log.Debug("lampshade TCP connection idled")
						})
					}
					return conn, err
				})
				return overheadWrapper(true)(conn, err)
			})
		}), nil
	}

	return newProxy(name, "lampshade", "tcp", s.Addr, s, uc, s.Trusted, dial)
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

type proxy struct {
	// Store int64's up front to ensure alignment of 64 bit words
	// See https://golang.org/pkg/sync/atomic/#pkg-note-BUG
	attempts          int64
	successes         int64
	consecSuccesses   int64
	failures          int64
	consecFailures    int64
	abe               int64 // Mbps scaled by 1000
	probeSuccesses    uint64
	probeSuccessKBs   uint64
	probeFailures     uint64
	probeFailedKBs    uint64
	dataSent          uint64
	dataRecv          uint64
	consecRWSuccesses consecCounter
	name              string
	protocol          string
	network           string
	addr              string
	authToken         string
	user              common.UserConfig
	trusted           bool
	bias              int
	doDialServer      func(context.Context, *proxy) (serverConn, error)
	emaRTT            *ema.EMA
	emaRTTDev         *ema.EMA
	emaSuccessRate    *ema.EMA
	kcpConfig         *KCPConfig
	forceRedial       *abool.AtomicBool
	mostRecentABETime time.Time
	doDialCore        func(ctx context.Context) (net.Conn, time.Duration, error)
	preconnects       chan interface{}
	preconnected      chan *proxyConnection
	closeCh           chan bool
	closeOnce         sync.Once
	mx                sync.Mutex
}

func newProxy(name, protocol, network, addr string, s *ChainedServerInfo, uc common.UserConfig, trusted bool, dialServer func(context.Context, *proxy) (serverConn, error)) (*proxy, error) {
	initPreconnect := s.InitPreconnect
	if initPreconnect <= 0 {
		initPreconnect = defaultInitPreconnect
	}
	maxPreconnect := s.MaxPreconnect
	if maxPreconnect <= 0 {
		maxPreconnect = defaultMaxPreconnect
	}

	p := &proxy{
		name:            name,
		protocol:        protocol,
		network:         network,
		addr:            addr,
		authToken:       s.AuthToken,
		user:            uc,
		trusted:         trusted,
		bias:            s.Bias,
		doDialServer:    dialServer,
		emaRTT:          ema.NewDuration(0, rttAlpha),
		emaRTTDev:       ema.NewDuration(0, rttDevAlpha),
		emaSuccessRate:  ema.New(1, SuccessRateAlpha), // Consider a proxy success when initializing
		forceRedial:     abool.New(),
		preconnects:     make(chan interface{}, maxPreconnect),
		preconnected:    make(chan *proxyConnection, maxPreconnect),
		closeCh:         make(chan bool, 1),
		consecSuccesses: 1, // be optimistic
	}

	if s.Bias == 0 && s.ENHTTPURL != "" {
		// By default, do not prefer ENHTTP proxies. Use a very low bias as domain-
		// fronting is our very-last resort.
		p.bias = -10
	}

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()
		conn, err := netx.DialTimeout("tcp", p.addr, timeoutFor(ctx))
		delta := elapsed()
		log.Tracef("Core dial time to %v was %v", p.Name(), delta)
		return conn, delta, err
	}

	if s.KCPSettings != nil && len(s.KCPSettings) > 0 {
		err := enableKCP(p, s)
		if err != nil {
			return nil, err
		}
		p.protocol = "kcp"
	}

	log.Debugf("%v preconnects, init: %d   max: %d", p.Label(), initPreconnect, maxPreconnect)
	p.processPreconnects(initPreconnect)
	return p, nil
}

func enableKCP(p *proxy, s *ChainedServerInfo) error {
	var cfg KCPConfig
	err := mapstructure.Decode(s.KCPSettings, &cfg)
	if err != nil {
		return log.Errorf("Could not decode kcp transport settings?: %v", err)
	}
	p.kcpConfig = &cfg

	// Fix address (comes across as kcp-placeholder)
	p.addr = cfg.RemoteAddr
	// KCP consumes a lot of bandwidth, so we want to bias against using it unless
	// everything else is blocked. However, we prefer it to domain-fronting. We
	// only default the bias if none was configured.
	if p.bias == 0 {
		p.bias = -1
	}

	addIdleTiming := func(conn net.Conn) net.Conn {
		log.Debug("Wrapping KCP with idletiming")
		return idletiming.Conn(conn, IdleTimeout*2, func() {
			log.Debug("KCP connection idled")
		})
	}
	dialKCP := kcpwrapper.Dialer(&cfg.DialerConfig, addIdleTiming)
	var dialKCPMutex sync.Mutex

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()

		dialKCPMutex.Lock()
		if p.forceRedial.IsSet() {
			log.Debug("Connection state changed, re-connecting to server first")
			dialKCP = kcpwrapper.Dialer(&p.kcpConfig.DialerConfig, addIdleTiming)
			p.forceRedial.UnSet()
		}
		doDialKCP := dialKCP
		dialKCPMutex.Unlock()

		conn, err := doDialKCP(ctx, "tcp", p.addr)
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}

func (p *proxy) ForceRedial() {
	p.forceRedial.Set()
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

func (p *proxy) Trusted() bool {
	return p.trusted
}

func (p *proxy) AdaptRequest(req *http.Request) {
	req.Header.Add(common.TokenHeader, p.authToken)
}

func (p *proxy) dialServer() (serverConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainedDialTimeout)
	defer cancel()
	return p.doDialServer(ctx, p)
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
	// Take deviation into account, see rfc6298
	return time.Duration(p.emaRTT.Get() + rttDevK*p.emaRTTDev.Get())
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

func (p *proxy) setStats(attempts int64, successes int64, consecSuccesses int64, failures int64, consecFailures int64, emaRTT time.Duration, mostRecentABETime time.Time, abe int64) {
	p.mx.Lock()
	atomic.StoreInt64(&p.attempts, attempts)
	atomic.StoreInt64(&p.successes, successes)
	atomic.StoreInt64(&p.consecSuccesses, consecSuccesses)
	atomic.StoreInt64(&p.failures, failures)
	atomic.StoreInt64(&p.consecFailures, consecFailures)
	p.emaRTT.SetDuration(emaRTT)
	p.mostRecentABETime = mostRecentABETime
	atomic.StoreInt64(&p.abe, abe)
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

func (p *proxy) dialCore(op *ops.Op) func(ctx context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		estRTT, estBandwidth := p.EstRTT(), p.EstBandwidth()
		if estRTT > 0 {
			op.SetMetricAvg("est_rtt", estRTT.Seconds()/1000)
		}
		if estBandwidth > 0 {
			op.SetMetricAvg("est_mbps", estBandwidth)
		}
		conn, delta, err := p.doDialCore(ctx)
		op.CoreDialTime(delta, err)
		return overheadWrapper(false)(conn, err)
	}
}

// KCPConfig adapts kcpwrapper.DialerConfig to the currently deployed
// configurations in order to provide backward-compatibility.
type KCPConfig struct {
	kcpwrapper.DialerConfig `mapstructure:",squash"`
	RemoteAddr              string `json:"remoteaddr"`
}

func timeoutFor(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		return deadline.Sub(time.Now())
	}
	return chainedDialTimeout
}

func (p *proxy) reportedDial(addr, protocol, network string, dial func(op *ops.Op) (net.Conn, error)) (serverConn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(p.Name(), addr, protocol, network)
	defer op.End()

	elapsed := mtime.Stopwatch()
	conn, err := dial(op)
	delta := elapsed()
	op.DialTime(delta, err)
	reportProxyDial(delta, err)

	return p.defaultServerConn(conn, op.FailIf(err))
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

func tlsConfigForProxy(s *ChainedServerInfo) (*tls.Config, tls.ClientHelloID) {
	var sessionCache tls.ClientSessionCache
	if s.TLSClientSessionCacheSize == 0 {
		sessionCache = tls.NewLRUClientSessionCache(1000)
	} else if s.TLSClientSessionCacheSize > 0 {
		sessionCache = tls.NewLRUClientSessionCache(s.TLSClientSessionCacheSize)
	}
	cipherSuites := orderedCipherSuitesFromConfig(s)

	return &tls.Config{
		ClientSessionCache: sessionCache,
		CipherSuites:       cipherSuites,
		ServerName:         s.TLSServerNameIndicator,
		InsecureSkipVerify: true,
	}, s.clientHelloID()
}

func orderedCipherSuitesFromConfig(s *ChainedServerInfo) []uint16 {
	if runtime.GOOS == "android" {
		return s.mobileOrderedCipherSuites()
	}
	return s.desktopOrderedCipherSuites()
}
