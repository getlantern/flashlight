package chained

import (
	"context"
	"crypto/rsa"
	gtls "crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"
	"github.com/mitchellh/mapstructure"
	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/cmux"
	"github.com/getlantern/ema"
	"github.com/getlantern/enhttp"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/kcpwrapper"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lampshade"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/quicwrapper"
	"github.com/getlantern/tinywss"
	"github.com/getlantern/tlsdialer"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained/config"
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

var (
	chainedDialTimeout          = 1 * time.Minute
	theForceAddr, theForceToken string

	tlsKeyLogWriter        io.Writer
	createKeyLogWriterOnce sync.Once
)

// CreateDialer creates a Proxy (balancer.Dialer) with supplied server info.
func CreateDialer(name string, s *ChainedServerInfo, uc common.UserConfig) (balancer.Dialer, error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	if s.Addr == "" {
		return nil, errors.New("Empty addr")
	}
	transport := s.PluggableTransport
	proto := "tcp"

	switch transport {
	case "", "http", "https", "utphttp", "utphttps":
		transport := "http"
		if strings.HasPrefix(s.PluggableTransport, "utp") {
			proto = "udp"
			transport = "utphttp"
		}
		var p *proxy
		var err error
		if s.Cert == "" {
			log.Errorf("No Cert configured for %s, will dial with plain tcp", s.Addr)
			p, err = newHTTPProxy(name, transport, proto, s, uc)
		} else if len(s.KCPSettings) > 0 {
			log.Errorf("KCP configured for %s, not using tls", s.Addr)
			p, err = newHTTPProxy(name, transport, proto, s, uc)
		} else {
			transport = transport + "s"
			log.Tracef("Cert configured for %s, will dial with tls", s.Addr)
			p, err = newHTTPSProxy(name, transport, proto, s, uc)
		}
		return p, err
	case "obfs4", "utpobfs4":
		return newOBFS4Proxy(name, transport, proto, s, uc)
	case "lampshade":
		return newLampshadeProxy(name, transport, proto, s, uc)
	case "quic", "oquic":
		return newQUICProxy(name, s, uc)
	case "wss":
		return newWSSProxy(name, s, uc)
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

func newHTTPProxy(name, transport, proto string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	doDialServer := func(ctx context.Context, p *proxy) (net.Conn, error) {
		return p.reportedDial(p.addr, p.protocol, p.network, func(op *ops.Op) (net.Conn, error) {
			return p.dialCore(op)(ctx)
		})
	}

	dialOrigin := defaultDialOrigin
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
		dialOrigin = func(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error) {
			dfConn, err := dial(network, addr)
			if err == nil {
				dfConn = idletiming.Conn(dfConn, IdleTimeout, func() {
					log.Debug("enhttp connection idled")
				})
			}
			return dfConn, err
		}
	}
	return newProxy(name, transport, proto, s, uc, s.ENHTTPURL != "", true, doDialServer, dialOrigin)
}

func newHTTPSProxy(name, transport, proto string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	x509cert := cert.X509()

	doDialServer := func(ctx context.Context, p *proxy) (net.Conn, error) {
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
	}
	return newProxy(name, transport, proto, s, uc, s.Trusted, true, doDialServer, defaultDialOrigin)
}

func newOBFS4Proxy(name, transport, proto string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
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

	doDialServer := func(ctx context.Context, p *proxy) (net.Conn, error) {
		return p.reportedDial(p.Addr(), p.Protocol(), p.Network(), func(op *ops.Op) (net.Conn, error) {
			dialFn := func(network, address string) (net.Conn, error) {
				// We know for sure the network and address are the same as what
				// the inner DailServer uses.
				return p.dialCore(op)(ctx)
			}

			// The proxy it wrapped already has timeout applied.
			return overheadWrapper(true)(cf.Dial("tcp", p.addr, dialFn, args))
		})
	}
	return newProxy(name, transport, proto, s, uc, s.Trusted, true, doDialServer, defaultDialOrigin)
}

func newLampshadeProxy(name, transport, proto string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	rsaPublicKey, ok := cert.X509().PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not an RSA public key")
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
	pingInterval, parseErr := time.ParseDuration(s.ptSetting("pinginterval"))
	if parseErr != nil || pingInterval < 0 {
		pingInterval = 15 * time.Second
		log.Debugf("%s: defaulted pinginterval to %v", name, pingInterval)
	}
	liveConns := s.ptSettingInt("liveconns")
	if liveConns <= 0 {
		liveConns = 2
		log.Debugf("%s: defaulted liveconns to %v", name, liveConns)
	}

	longDialTimeout := s.ptSettingInt("longdialtimeout")
	shortDialTimeout := s.ptSettingInt("shortdialtimeout")
	dialer := lampshade.NewDialer(&lampshade.DialerOpts{
		WindowSize:        windowSize,
		MaxPadding:        maxPadding,
		LiveConns:         liveConns,
		MaxStreamsPerConn: maxStreamsPerConn,
		PingInterval:      pingInterval,
		Pool:              buffers.Pool,
		Cipher:            cipherCode,
		ServerPublicKey:   rsaPublicKey,
		Dial: func(timeout time.Duration) (net.Conn, error) {
			start := time.Now()
			// note - we do not wrap the TCP connection with IdleTiming because
			// lampshade cleans up after itself and won't leave excess unused
			// connections hanging around.
			log.Debugf("Dialing lampshade TCP connection to %v", name)
			conn, err := netx.DialTimeout("tcp", s.Addr, timeout)
			elapsed := time.Since(start)
			if err != nil {
				log.Errorf("Could not dial TCP connection to %v after %v: %v", name, elapsed, err)
			} else {
				log.Debugf("Successfully created lampshade TCP connection to %v in %v", name, elapsed)
			}
			return conn, err
		},
		Lifecycle:        newLampshadeLifecycleListener(name),
		Name:             name,
		LongDialTimeout:  longDialTimeout,
		ShortDialTimeout: shortDialTimeout,
	})
	doDialServer := func(ctx context.Context, p *proxy) (net.Conn, error) {
		return p.reportedDial(s.Addr, transport, proto, func(op *ops.Op) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx)
			return overheadWrapper(true)(conn, err)
		})
	}
	return newProxy(name, transport, proto, s, uc, s.Trusted, false, doDialServer, defaultDialOrigin)
}

func newQUICProxy(name string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {

	dialServer := func(ctx context.Context, p *proxy) (net.Conn, error) {
		return p.reportedDial(s.Addr, "quic", "udp", func(op *ops.Op) (net.Conn, error) {
			conn, err := p.dialCore(op)(ctx)
			return overheadWrapper(true)(conn, err)
		})
	}

	return newProxy(name, "quic", "udp", s, uc, s.Trusted, false, dialServer, defaultDialOrigin)
}

func newWSSProxy(name string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {

	doDialServer := func(ctx context.Context, p *proxy) (net.Conn, error) {
		return p.reportedDial(p.addr, p.protocol, p.network, func(op *ops.Op) (net.Conn, error) {
			conn, err := p.dialCore(op)(ctx)
			return overheadWrapper(true)(conn, err)
		})
	}

	return newProxy(name, "wss", "tcp", s, uc, s.Trusted, false, doDialServer, defaultDialOrigin)
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

type dialServerFn func(context.Context, *proxy) (net.Conn, error)

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
	doDialServer        dialServerFn
	dialOrigin          dialOriginFn
	emaRTT              *ema.EMA
	emaRTTDev           *ema.EMA
	emaSuccessRate      *ema.EMA
	kcpConfig           *KCPConfig
	mostRecentABETime   time.Time
	doDialCore          func(ctx context.Context) (net.Conn, time.Duration, error)
	numPreconnecting    func() int
	numPreconnected     func() int
	closeCh             chan bool
	closeOnce           sync.Once
	mx                  sync.Mutex
}

func newProxy(name, protocol, network string, s *ChainedServerInfo, uc common.UserConfig, trusted bool, allowPreconnecting bool, dialServer dialServerFn, dialOrigin dialOriginFn) (*proxy, error) {
	addr := s.Addr
	multiplexed := false
	if s.MultiplexedAddr != "" {
		addr = s.MultiplexedAddr
		multiplexed = true
	}

	p := &proxy{
		name:             name,
		protocol:         protocol,
		network:          network,
		multiplexed:      multiplexed,
		addr:             addr,
		location:         s.Location,
		authToken:        s.AuthToken,
		user:             uc,
		trusted:          trusted,
		bias:             s.Bias,
		doDialServer:     dialServer,
		dialOrigin:       dialOrigin,
		emaRTT:           ema.NewDuration(0, rttAlpha),
		emaRTTDev:        ema.NewDuration(0, rttDevAlpha),
		emaSuccessRate:   ema.New(1, successRateAlpha), // Consider a proxy success when initializing
		numPreconnecting: func() int { return 0 },
		numPreconnected:  func() int { return 0 },
		closeCh:          make(chan bool, 1),
		consecSuccesses:  1, // be optimistic
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

	if s.MultiplexedAddr != "" || s.PluggableTransport == "utphttp" || s.PluggableTransport == "utphttps" || s.PluggableTransport == "utpobfs4" {
		log.Debugf("Enabling multiplexing for %v", p.Label())
		origDoDialServer := p.doDialServer
		poolSize := s.MultiplexedPhysicalConns
		if poolSize < 1 {
			poolSize = defaultMultiplexedPhysicalConns
		}
		multiplexedDial := cmux.Dialer(&cmux.DialerOpts{
			Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return origDoDialServer(ctx, p)
			},
			KeepAliveInterval: IdleTimeout / 2,
			KeepAliveTimeout:  IdleTimeout,
			PoolSize:          poolSize,
		})
		p.doDialServer = func(ctx context.Context, p *proxy) (net.Conn, error) {
			return multiplexedDial(ctx, "", "")
		}
	}
	if len(s.KCPSettings) > 0 {
		log.Debugf("Enabling KCP for %v (%v)", p.Label(), p.protocol)
		err := enableKCP(p, s)
		if err != nil {
			return nil, err
		}
		p.protocol = "kcp"
	} else if s.PluggableTransport == "quic" || s.PluggableTransport == "oquic" {
		log.Debugf("Enabling QUIC for %v (%v)", p.Label(), p.protocol)
		err := enableQUIC(p, s)
		if err != nil {
			return nil, err
		}
		p.protocol = s.PluggableTransport
	} else if strings.HasPrefix(s.PluggableTransport, "utp") {
		log.Debugf("Enabling UTP for %v (%v)", p.Label(), p.protocol)
		err := enableUTP(p, s)
		if err != nil {
			return nil, err
		}
		p.protocol = "utp"
	} else if s.PluggableTransport == "wss" {
		log.Debugf("Enabling WSS for %v (%v)", p.Label(), p.protocol)
		err := enableWSS(p, s)
		if err != nil {
			return nil, err
		}
		p.protocol = "wss"
	} else if allowPreconnecting && s.MaxPreconnect > 0 {
		log.Debugf("Enabling preconnecting for %v", p.Label())
		// give ourselves a large margin for making sure we're not using idled preconnected connections
		expiration := IdleTimeout / 2
		pd := newPreconnectingDialer(name, s.MaxPreconnect, expiration, p.closeCh, p.doDialServer)
		p.doDialServer = pd.dial
		p.numPreconnecting = pd.numPreconnecting
		p.numPreconnected = pd.numPreconnected
	}

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

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()

		conn, err := dialKCP(ctx, "tcp", p.addr)
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}

func enableWSS(p *proxy, s *ChainedServerInfo) error {
	var rt tinywss.RoundTripHijacker
	var err error

	fctx_id := s.ptSetting("df_ctx")
	if fctx_id != "" {
		fctx := GetFrontingContext(fctx_id)
		if fctx == nil {
			return fmt.Errorf("unsupported wss df_ctx=%s! skipping", fctx_id)
		}
		timeout, err := time.ParseDuration(s.ptSetting("df_timeout"))
		if err != nil || timeout < 0 {
			timeout = 1 * time.Minute
		}
		log.Debugf("Using wss fctx_id=%s timeout=%v", fctx_id, timeout)
		rt = &wssFrontedRT{fctx, timeout}
	} else {
		log.Debugf("Using wss https direct")
		rt, err = wssHTTPSRoundTripper(p, s)
		if err != nil {
			return err
		}
	}

	opts := &tinywss.ClientOpts{
		URL:               fmt.Sprintf("wss://%s", p.addr),
		RoundTrip:         rt,
		KeepAliveInterval: IdleTimeout / 2,
		KeepAliveTimeout:  IdleTimeout,
		Multiplexed:       s.ptSettingBool("multiplexed"),
		MaxFrameSize:      s.ptSettingInt("max_frame_size"),
		MaxReceiveBuffer:  s.ptSettingInt("max_receive_buffer"),
	}

	client := tinywss.NewClient(opts)

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()
		conn, err := client.DialContext(ctx)
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}

type wssFrontedRT struct {
	fctx    *fronted.FrontingContext
	timeout time.Duration
}

func (rt *wssFrontedRT) RoundTripHijack(req *http.Request) (*http.Response, net.Conn, error) {
	r, ok := rt.fctx.NewDirect(rt.timeout)
	if !ok {
		return nil, nil, fmt.Errorf("unable to obtain fronted roundtripper after %v fctx=%s", rt.timeout, rt.fctx)
	}
	if rth, ok := r.(tinywss.RoundTripHijacker); ok {
		return rth.RoundTripHijack(req)
	}
	return nil, nil, fmt.Errorf("unsupported roundtripper obtained from fronted")
}

func wssHTTPSRoundTripper(p *proxy, s *ChainedServerInfo) (tinywss.RoundTripHijacker, error) {

	// Verify the SNI name if given, otherwise verify the hostname given and do not use SNI.
	var err error
	serverName := s.TLSServerNameIndicator
	sendServerName := true
	if serverName == "" {
		sendServerName = false
		serverName, _, err = net.SplitHostPort(s.Addr)
		if err != nil {
			serverName = s.Addr
		}
	}
	helloID := s.clientHelloID()
	pinnedCert := s.ptSettingBool("pin_certificate")

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	x509cert := cert.X509()

	return tinywss.NewRoundTripper(func(network, addr string) (net.Conn, error) {
		log.Debugf("tinywss Roundtripper dialing %v", addr)

		var certPool *x509.CertPool

		if !pinnedCert {
			certPool = GetFrontingCertPool(1 * time.Second)
			if certPool == nil {
				log.Debugf("wss cert pool is not available (yet?), falling back to pinned.")
			}
		}

		if certPool == nil {
			certPool = x509.NewCertPool()
			certPool.AddCert(x509cert)
			log.Debugf("wss using pinned certificate")
		}

		tlsConf := &tls.Config{
			CipherSuites: orderedCipherSuitesFromConfig(s),
			ServerName:   serverName,
			RootCAs:      certPool,
			KeyLogWriter: getTLSKeyLogWriter(),
		}

		td := &tlsdialer.Dialer{
			DoDial:         netx.DialTimeout,
			SendServerName: sendServerName,
			Config:         tlsConf,
			ClientHelloID:  helloID,
			Timeout:        chainedDialTimeout,
		}

		return td.Dial(network, addr)
	}), nil
}

func enableQUIC(p *proxy, s *ChainedServerInfo) error {
	addr := s.Addr
	tlsConf := &gtls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}

	maxStreamsPerConn := s.ptSettingInt("streams")

	quicConf := &quicwrapper.Config{
		IdleTimeout:        IdleTimeout,
		MaxIncomingStreams: maxStreamsPerConn,
		KeepAlive:          true,
	}

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	pinnedCert := cert.X509()

	dialFn := quicwrapper.DialWithNetx

	if s.PluggableTransport == "oquic" {
		oquicKeyStr := s.ptSetting("oquic_key")
		if oquicKeyStr == "" {
			return log.Error("Missing oquic_key for oquic transport")
		}
		oquicKey, err := base64.StdEncoding.DecodeString(oquicKeyStr)
		if err != nil {
			return log.Error(errors.Wrap(err).With("oquic_key", oquicKeyStr))
		}
		oquicConfig := quicwrapper.DefaultOQuicConfig(oquicKey)
		if cipher := s.ptSetting("oquic_cipher"); cipher != "" {
			oquicConfig.Cipher = cipher
		}
		if s.ptSetting("oquic_aggressive_padding") != "" {
			oquicConfig.AggressivePadding = int64(s.ptSettingInt("oquic_aggressive_padding"))
		}
		if s.ptSetting("oquic_max_padding_hint") != "" {
			oquicConfig.MaxPaddingHint = uint8(s.ptSettingInt("oquic_max_padding_hint"))
		}
		if s.ptSetting("oquic_min_padded") != "" {
			oquicConfig.MinPadded = s.ptSettingInt("oquic_min_padded")
		}

		dialFn, err = quicwrapper.NewOQuicDialerWithUDPDialer(quicwrapper.DialUDPNetx, oquicConfig)
		if err != nil {
			return log.Errorf("Unable to create oquic dialer: %v", err)
		}
	}

	dialer := quicwrapper.NewClientWithPinnedCert(
		addr,
		tlsConf,
		quicConf,
		dialFn,
		pinnedCert,
	)
	// when the proxy closes, close the dialer
	go func() {
		<-p.closeCh
		log.Debug("Closing quic session: Proxy closed.")
		dialer.Close()
	}()

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()
		conn, err := dialer.DialContext(ctx)
		if err != nil {
			log.Debugf("Failed to establish multiplexed connection: %s", err)
		} else {
			log.Debug("established new multiplexed quic connection.")
		}
		delta := elapsed()
		return conn, delta, err
	}

	return nil
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

func (p *proxy) dialServer(ctx context.Context) (net.Conn, error) {
	conn, err := p.doDialServer(ctx, p)
	if err != nil {
		log.Errorf("Unable to dial server %v: %s", p.Label(), err)
		p.MarkFailure()
	}
	return conn, err
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

func (p *proxy) dialCore(op *ops.Op) func(ctx context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		estRTT, estBandwidth := p.EstRTT(), p.EstBandwidth()
		if estRTT > 0 {
			op.SetMetricAvg("est_rtt_ms", estRTT.Seconds()*1000)
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
		return time.Until(deadline)
	}
	return chainedDialTimeout
}

func (p *proxy) reportedDial(addr, protocol, network string, dial func(op *ops.Op) (net.Conn, error)) (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(p.Name(), addr, protocol, network, p.multiplexed)
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
		KeyLogWriter:       getTLSKeyLogWriter(),
	}, s.clientHelloID()
}

func orderedCipherSuitesFromConfig(s *ChainedServerInfo) []uint16 {
	if common.Platform == "android" {
		return s.mobileOrderedCipherSuites()
	}
	return s.desktopOrderedCipherSuites()
}

// Write the session keys to file if SSLKEYLOGFILE is set, same as browsers.
func getTLSKeyLogWriter() io.Writer {
	createKeyLogWriterOnce.Do(func() {
		path := os.Getenv("SSLKEYLOGFILE")
		if path == "" {
			return
		}
		var err error
		tlsKeyLogWriter, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			log.Debugf("Error creating keylog file at %v: %s", path, err)
		}
	})
	return tlsKeyLogWriter
}
