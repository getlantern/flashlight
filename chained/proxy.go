package chained

import (
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"

	"github.com/getlantern/ema"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/kcptun/client/lib"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lampshade"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsdialer"
	"github.com/mitchellh/mapstructure"
)

const (
	trustedSuffix = " (t)"
)

var (
	chainedDialTimeout          = 10 * time.Second
	theForceAddr, theForceToken string
)

// CreateDialer creates a Proxy (balancer.Dialer) with supplied server info.
func CreateDialer(name string, s *ChainedServerInfo, deviceID string, proToken func() string) (balancer.Dialer, error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	if s.Addr == "" {
		return nil, errors.New("Empty addr")
	}
	if s.AuthToken == "" {
		return nil, errors.New("No auth token").With("addr", s.Addr)
	}
	switch s.PluggableTransport {
	case "":
		var p *proxy
		var err error
		if s.Cert == "" {
			log.Errorf("No Cert configured for %s, will dial with plain tcp", s.Addr)
			p = newHTTPProxy(name, s, deviceID, proToken)
		} else {
			log.Tracef("Cert configured for  %s, will dial with tls", s.Addr)
			p, err = newHTTPSProxy(name, s, deviceID, proToken)
		}
		return p, err
	case "obfs4":
		return newOBFS4Proxy(name, s, deviceID, proToken)
	case "lampshade":
		return newLampshadeProxy(name, s, deviceID, proToken)
	case "kcp":
		return newKCPProxy(name, s, deviceID, proToken)
	default:
		return nil, errors.New("Unknown transport").With("addr", s.Addr).With("plugabble-transport", s.PluggableTransport)
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

func newHTTPProxy(name string, s *ChainedServerInfo, deviceID string, proToken func() string) *proxy {
	return newProxy(name, "http", "tcp", s.Addr, s, deviceID, proToken, false, func(p *proxy) (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(p.addr, p.protocol, p.network)
		defer op.End()
		elapsed := mtime.Stopwatch()
		conn, err := p.tcpDial(op)(chainedDialTimeout)
		p.dialTime(op, elapsed, err)
		return conn, op.FailIf(err)
	})
}

func newHTTPSProxy(name string, s *ChainedServerInfo, deviceID string, proToken func() string) (*proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	x509cert := cert.X509()
	sessionCache := tls.NewLRUClientSessionCache(1000)
	return newProxy(name, "https", "tcp", s.Addr, s, deviceID, proToken, s.Trusted, func(p *proxy) (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(p.addr, p.protocol, p.network)
		defer op.End()

		elapsed := mtime.Stopwatch()
		conn, err := tlsdialer.DialTimeout(func(network, addr string, timeout time.Duration) (net.Conn, error) {
			return p.tcpDial(op)(timeout)
		}, chainedDialTimeout,
			"tcp", p.addr, false, &tls.Config{
				ClientSessionCache: sessionCache,
				InsecureSkipVerify: true,
			})
		p.dialTime(op, elapsed, err)
		if err != nil {
			return nil, op.FailIf(err)
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
	}), nil
}

func newKCPProxy(name string, s *ChainedServerInfo, deviceID string, proToken func() string) (*proxy, error) {
	var conf lib.Config
	err := mapstructure.Decode(s.PluggableTransportSettings, &conf)
	if err != nil {
		log.Errorf("Could not decode pluggable transport settings?")
		return nil, err
	}
	go func() {
		lib.Run(&conf, "4.1.4")
	}()

	util.WaitForServer(conf.LocalAddr)
	return newProxy(name, "kcp", "udp", conf.LocalAddr, s, deviceID, proToken, s.Trusted, func(p *proxy) (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(p.Addr(), p.Protocol(), p.Network())
		defer op.End()
		elapsed := mtime.Stopwatch()
		// The proxy it wrapped already has timeout applied.
		conn, err := p.tcpDial(op)(chainedDialTimeout)
		p.dialTime(op, elapsed, err)
		return conn, op.FailIf(err)
	}), nil
}

func newOBFS4Proxy(name string, s *ChainedServerInfo, deviceID string, proToken func() string) (*proxy, error) {
	if s.Cert == "" {
		return nil, fmt.Errorf("No Cert configured for obfs4 server, can't connect")
	}

	cf, err := (&obfs4.Transport{}).ClientFactory("")
	if err != nil {
		return nil, log.Errorf("Unable to create obfs4 client factory: %v", err)
	}

	ptArgs := &pt.Args{}
	ptArgs.Add("cert", s.Cert)
	ptArgs.Add("iat-mode", s.PluggableTransportSettings["iat-mode"])

	args, err := cf.ParseArgs(ptArgs)
	if err != nil {
		return nil, log.Errorf("Unable to parse client args: %v", err)
	}

	return newProxy(name, "obfs4", "tcp", s.Addr, s, deviceID, proToken, s.Trusted, func(p *proxy) (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(p.Addr(), p.Protocol(), p.Network())
		defer op.End()
		elapsed := mtime.Stopwatch()
		dialFn := func(network, address string) (net.Conn, error) {
			// We know for sure the network and address are the same as what
			// the inner DailServer uses.
			return p.tcpDial(op)(chainedDialTimeout)
		}
		// The proxy it wrapped already has timeout applied.
		conn, err := cf.Dial("tcp", p.addr, dialFn, args)
		p.dialTime(op, elapsed, err)
		return overheadWrapper(true)(conn, op.FailIf(err))
	}), nil
}

func newLampshadeProxy(name string, s *ChainedServerInfo, deviceID string, proToken func() string) (*proxy, error) {
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
	dial := func(p *proxy) (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(s.Addr, "lampshade", "tcp").
			Set("ls_win", windowSize).
			Set("ls_pad", maxPadding).
			Set("ls_streams", int(maxStreamsPerConn)).
			Set("ls_cipher", cipherCode.String())
		defer op.End()

		elapsed := mtime.Stopwatch()
		conn, err := dialer.Dial(func() (net.Conn, error) {
			conn, err := p.tcpDial(op)(chainedDialTimeout)
			if err == nil && idleInterval > 0 {
				conn = idletiming.Conn(conn, idleInterval, func() {
					log.Debug("lampshade TCP connection idled")
				})
			}
			return conn, err
		})
		// note - because lampshade is multiplexed, this dial time will often be
		// lower than other protocols since there's often nothing to be done for
		// opening up a new multiplexed connection.
		p.dialTime(op, elapsed, err)
		return overheadWrapper(true)(conn, op.FailIf(err))
	}

	p := newProxy(name, "lampshade", "tcp", s.Addr, s, deviceID, proToken, s.Trusted, dial)

	if pingInterval > 0 {
		go func() {
			for {
				time.Sleep(pingInterval * 2)
				ttfa := dialer.EMARTT()
				p.emaLatency.SetDuration(ttfa)
				log.Debugf("%v EMA RTT: %v", p.Label(), ttfa)
			}
		}()
	}

	return p, nil
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
	bbrResetRequired  int64
	name              string
	protocol          string
	network           string
	addr              string
	authToken         string
	deviceID          string
	proToken          func() string
	trusted           bool
	dialServer        func(*proxy) (net.Conn, error)
	emaDialTime       *ema.EMA
	emaLatency        *ema.EMA
	mostRecentABETime time.Time
	forceRecheckCh    chan bool
	closeCh           chan bool
	mx                sync.Mutex
}

func newProxy(name, protocol, network, addr string, s *ChainedServerInfo, deviceID string, proToken func() string, trusted bool, dialServer func(*proxy) (net.Conn, error)) *proxy {
	p := &proxy{
		name:             name,
		protocol:         protocol,
		network:          network,
		addr:             addr,
		authToken:        s.AuthToken,
		deviceID:         deviceID,
		proToken:         proToken,
		trusted:          trusted,
		dialServer:       dialServer,
		emaDialTime:      ema.NewDuration(0, 0.8),
		emaLatency:       ema.NewDuration(0, 0.8),
		bbrResetRequired: 1, // reset on every start
		forceRecheckCh:   make(chan bool, 1),
		closeCh:          make(chan bool, 1),
		consecSuccesses:  1, // be optimistic
	}
	go p.runConnectivityChecks()
	return p
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

func (p *proxy) DialServer() (net.Conn, error) {
	return p.dialServer(p)
}

func (p *proxy) dialTime(op *ops.Op, elapsed func() time.Duration, err error) {
	delta := elapsed()
	op.DialTime(delta, err)
	if err == nil {
		p.emaDialTime.UpdateDuration(delta)
	}
}

func (p *proxy) EMADialTime() time.Duration {
	return p.emaLatency.GetDuration()
}

func (p *proxy) EstLatency() time.Duration {
	return p.emaLatency.GetDuration()
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
	return float64(atomic.LoadInt64(&p.abe)) / 1000
}

func (p *proxy) setStats(attempts int64, successes int64, consecSuccesses int64, failures int64, consecFailures int64, emaLatency time.Duration, mostRecentABETime time.Time, abe int64) {
	p.mx.Lock()
	atomic.StoreInt64(&p.attempts, attempts)
	atomic.StoreInt64(&p.successes, successes)
	atomic.StoreInt64(&p.consecSuccesses, consecSuccesses)
	atomic.StoreInt64(&p.failures, failures)
	atomic.StoreInt64(&p.consecFailures, consecFailures)
	p.emaLatency.SetDuration(emaLatency)
	p.mostRecentABETime = mostRecentABETime
	p.abe = abe
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

func (p *proxy) shouldResetBBR() bool {
	return atomic.CompareAndSwapInt64(&p.bbrResetRequired, 1, 0)
}

func (p *proxy) forceRecheck() {
	select {
	case p.forceRecheckCh <- true:
		// requested
	default:
		// recheck already requested, ignore
	}
}

func (p *proxy) tcpDial(op *ops.Op) func(timeout time.Duration) (net.Conn, error) {
	return func(timeout time.Duration) (net.Conn, error) {
		estLatency, estBandwidth := p.EstLatency(), p.EstBandwidth()
		if estLatency > 0 {
			op.Set("est_rtt", estLatency.Seconds()/1000)
		}
		if estBandwidth > 0 {
			op.Set("est_mbps", estBandwidth)
		}
		conn, delta, err := p.dialTCP(timeout)
		op.TCPDialTime(delta, err)
		return overheadWrapper(false)(conn, err)
	}
}

func (p *proxy) dialTCP(timeout time.Duration) (net.Conn, time.Duration, error) {
	elapsed := mtime.Stopwatch()
	conn, err := netx.DialTimeout("tcp", p.addr, timeout)
	delta := elapsed()
	if err == nil {
		p.mx.Lock()
		p.emaLatency.UpdateDuration(delta)
		p.mx.Unlock()
	}
	return conn, delta, err
}
