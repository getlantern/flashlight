package proxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	rclient "github.com/go-redis/redis/v8"

	"github.com/getlantern/cmux/v2"
	"github.com/getlantern/cmuxprivate"
	"github.com/getlantern/enhttp"
	"github.com/getlantern/errors"
	"github.com/getlantern/geo"
	"github.com/getlantern/golog"
	"github.com/getlantern/gonat"
	"github.com/getlantern/http-proxy-lantern/v2/otel"
	shadowsocks "github.com/getlantern/http-proxy-lantern/v2/shadowsocks"
	"github.com/getlantern/kcpwrapper"

	"github.com/xtaci/smux"

	"github.com/getlantern/multipath"
	packetforward "github.com/getlantern/packetforward/server"
	"github.com/getlantern/proxy/v2"
	"github.com/getlantern/proxy/v2/filters"
	"github.com/getlantern/psmux"
	"github.com/getlantern/quicwrapper"
	"github.com/getlantern/tinywss"
	"github.com/getlantern/tlsdefaults"

	"github.com/getlantern/http-proxy/listeners"
	"github.com/getlantern/http-proxy/proxyfilters"
	"github.com/getlantern/http-proxy/server"

	"github.com/getlantern/http-proxy-lantern/v2/analytics"
	"github.com/getlantern/http-proxy-lantern/v2/bbr"
	"github.com/getlantern/http-proxy-lantern/v2/blacklist"
	"github.com/getlantern/http-proxy-lantern/v2/cleanheadersfilter"
	"github.com/getlantern/http-proxy-lantern/v2/devicefilter"
	"github.com/getlantern/http-proxy-lantern/v2/diffserv"
	"github.com/getlantern/http-proxy-lantern/v2/domains"
	"github.com/getlantern/http-proxy-lantern/v2/googlefilter"
	"github.com/getlantern/http-proxy-lantern/v2/httpsupgrade"
	"github.com/getlantern/http-proxy-lantern/v2/instrument"
	"github.com/getlantern/http-proxy-lantern/v2/lampshade"
	lanternlisteners "github.com/getlantern/http-proxy-lantern/v2/listeners"
	"github.com/getlantern/http-proxy-lantern/v2/mimic"
	"github.com/getlantern/http-proxy-lantern/v2/obfs4listener"
	"github.com/getlantern/http-proxy-lantern/v2/opsfilter"
	"github.com/getlantern/http-proxy-lantern/v2/ping"
	"github.com/getlantern/http-proxy-lantern/v2/quic"
	"github.com/getlantern/http-proxy-lantern/v2/redis"
	"github.com/getlantern/http-proxy-lantern/v2/throttle"
	"github.com/getlantern/http-proxy-lantern/v2/tlslistener"
	"github.com/getlantern/http-proxy-lantern/v2/tlsmasq"
	"github.com/getlantern/http-proxy-lantern/v2/tokenfilter"
	"github.com/getlantern/http-proxy-lantern/v2/versioncheck"
	"github.com/getlantern/http-proxy-lantern/v2/wss"
)

const (
	timeoutToDialOriginSite = 10 * time.Second
)

var (
	log = golog.LoggerFor("lantern-proxy")

	proxyNameRegex = regexp.MustCompile(`(fp-([a-z0-9]+-)?([a-z0-9]+)-[0-9]{8}-[0-9]+)(-.+)?`)
)

// Proxy is an HTTP proxy.
type Proxy struct {
	TestingLocal                       bool
	HTTPAddr                           string
	HTTPMultiplexAddr                  string
	HoneycombSampleRate                int
	TeleportSampleRate                 int
	ExternalIP                         string
	CertFile                           string
	CfgSvrAuthToken                    string
	CfgSvrCacheClear                   time.Duration
	ConnectOKWaitsForUpstream          bool
	ENHTTPAddr                         string
	ENHTTPServerURL                    string
	ENHTTPReapIdleTime                 time.Duration
	EnableMultipath                    bool
	EnableReports                      bool
	HTTPS                              bool
	IdleTimeout                        time.Duration
	KeyFile                            string
	Track                              string
	Pro                                bool
	ProxiedSitesSamplePercentage       float64
	ProxiedSitesTrackingID             string
	ReportingRedisClient               *rclient.Client
	ThrottleRefreshInterval            time.Duration
	Token                              string
	TunnelPorts                        string
	Obfs4Addr                          string
	Obfs4MultiplexAddr                 string
	Obfs4Dir                           string
	Obfs4HandshakeConcurrency          int
	Obfs4MaxPendingHandshakesPerClient int
	Obfs4HandshakeTimeout              time.Duration
	KCPConf                            string
	Benchmark                          bool
	DiffServTOS                        int
	LampshadeAddr                      string
	LampshadeKeyCacheSize              int
	LampshadeMaxClientInitAge          time.Duration
	VersionCheck                       bool
	VersionCheckRange                  string
	VersionCheckRedirectURL            string
	VersionCheckRedirectPercentage     float64
	GoogleSearchRegex                  string
	GoogleCaptchaRegex                 string
	BlacklistMaxIdleTime               time.Duration
	BlacklistMaxConnectInterval        time.Duration
	BlacklistAllowedFailures           int
	BlacklistExpiration                time.Duration
	ProxyName                          string
	ProxyProtocol                      string
	BuildType                          string
	BBRUpstreamProbeURL                string
	QUICIETFAddr                       string
	QUICUseBBR                         bool
	WSSAddr                            string
	PacketForwardAddr                  string
	ExternalIntf                       string
	SessionTicketKeyFile               string
	FirstSessionTicketKey              string
	RequireSessionTickets              bool
	MissingTicketReaction              tlslistener.HandshakeReaction
	TLSListenerAllowTLS13              bool
	TLSMasqAddr                        string
	TLSMasqOriginAddr                  string
	TLSMasqSecret                      string
	TLSMasqTLSMinVersion               uint16
	TLSMasqTLSCipherSuites             []uint16
	ShadowsocksAddr                    string
	ShadowsocksMultiplexAddr           string
	ShadowsocksSecret                  string
	ShadowsocksCipher                  string
	ShadowsocksReplayHistory           int
	PromExporterAddr                   string
	CountryLookup                      geo.CountryLookup
	ISPLookup                          geo.ISPLookup

	MultiplexProtocol             string
	SmuxVersion                   int
	SmuxMaxFrameSize              int
	SmuxMaxReceiveBuffer          int
	SmuxMaxStreamBuffer           int
	PsmuxVersion                  int
	PsmuxMaxFrameSize             int
	PsmuxMaxReceiveBuffer         int
	PsmuxMaxStreamBuffer          int
	PsmuxDisablePadding           bool
	PsmuxMaxPaddingRatio          float64
	PsmuxMaxPaddedSize            int
	PsmuxDisableAggressivePadding bool
	PsmuxAggressivePadding        int
	PsmuxAggressivePaddingRatio   float64

	bm             bbr.Middleware
	throttleConfig throttle.Config
	instrument     instrument.Instrument
}

type listenerBuilderFN func(addr string) (net.Listener, error)

type addresses struct {
	obfs4          string
	obfs4Multiplex string
	http           string
	httpMultiplex  string
	lampshade      string
	tlsmasq        string
}

// ListenAndServe listens, serves and blocks.
func (p *Proxy) ListenAndServe(ctx context.Context) error {
	if p.CountryLookup == nil {
		log.Debugf("Maxmind not configured, will not report country data to prometheus or in bandwidth data")
		p.CountryLookup = geo.NoLookup{}
	}
	if p.ISPLookup == nil {
		log.Debugf("Maxmind not configured, will not report ISP data to prometheus")
		p.ISPLookup = geo.NoLookup{}
	}

	p.instrument = instrument.NoInstrument{}
	if p.PromExporterAddr == "" {
		log.Debugf("Not enabling prometheus export")
	} else {
		log.Debugf("Enabling prometheus export at %v", p.PromExporterAddr)
		prom := instrument.NewPrometheus(
			p.CountryLookup,
			p.ISPLookup,
			instrument.CommonLabels{
				BuildType:             p.BuildType,
				Protocol:              p.ProxyProtocol,
				SupportTLSResumption:  p.SessionTicketKeyFile != "",
				RequireTLSResumption:  p.RequireSessionTickets,
				MissingTicketReaction: p.MissingTicketReaction.Action(),
			})
		go func() {
			log.Debugf("Running Prometheus exporter at http://%s/metrics", p.PromExporterAddr)
			if err := prom.Run(p.PromExporterAddr); err != nil {
				log.Error(err)
			}
		}()
		p.instrument = prom
	}
	var onServerError func(conn net.Conn, err error)
	var onListenerError func(conn net.Conn, err error)
	if err := p.setupPacketForward(); err != nil {
		log.Errorf("Unable to set up packet forwarding, will continue to start up: %v", err)
	}
	p.setBenchmarkMode()
	p.bm = bbr.New()
	if p.BBRUpstreamProbeURL != "" {
		go p.bm.ProbeUpstream(p.BBRUpstreamProbeURL)
	}
	p.loadThrottleConfig()

	if p.ENHTTPAddr != "" {
		return p.ListenAndServeENHTTP()
	}

	// Only allow connections from remote IPs that are not blacklisted
	blacklist := p.createBlacklist()
	filterChain, dial, err := p.createFilterChain(blacklist)
	if err != nil {
		return err
	}

	if p.QUICIETFAddr != "" {
		filterChain = filterChain.Prepend(quic.NewMiddleware())
	}
	if p.WSSAddr != "" {
		filterChain = filterChain.Append(wss.NewMiddleware())
	}
	filterChain = filterChain.Prepend(opsfilter.New(p.bm))

	srv := server.New(&server.Opts{
		IdleTimeout: p.IdleTimeout,
		// Use the same buffer pool as lampshade for now but need to optimize later.
		BufferSource:             lampshade.BufferPool,
		Dial:                     dial,
		Filter:                   p.instrument.WrapFilter("proxy", filterChain),
		OKDoesNotWaitForUpstream: !p.ConnectOKWaitsForUpstream,
		OnError:                  p.instrument.WrapConnErrorHandler("proxy_serve", onServerError),
	})
	stopHoneycomb := p.configureHoneycomb()
	defer stopHoneycomb()

	stopTeleport := p.configureTeleport()
	defer stopTeleport()

	bwReporting := p.configureBandwidthReporting()
	// Throttle connections when signaled
	srv.AddListenerWrappers(lanternlisteners.NewBitrateListener, bwReporting.wrapper)

	allListeners := make([]net.Listener, 0)
	listenerProtocols := make([]string, 0)
	addListenerIfNecessary := func(proto, addr string, fn listenerBuilderFN) error {
		if addr == "" {
			return nil
		}
		l, err := fn(addr)
		if err != nil {
			return err
		}
		listenerProtocols = append(listenerProtocols, proto)
		// Although we include blacklist functionality, it's currently only used to
		// track potential blacklisting ad doesn't actually blacklist anyone.
		allListeners = append(allListeners, lanternlisteners.NewAllowingListener(l, blacklist.OnConnect))
		return nil
	}

	addListenersForBaseTransport := func(baseListen func(string, bool) (net.Listener, error), addrs *addresses) error {
		if err := addListenerIfNecessary("obfs4", addrs.obfs4, p.listenOBFS4(baseListen)); err != nil {
			return err
		}
		if err := addListenerIfNecessary("obfs4_multiplex", addrs.obfs4Multiplex, p.wrapMultiplexing(p.listenOBFS4(baseListen))); err != nil {
			return err
		}

		// We pass onListenerError to lampshade so that we can count errors in its
		// internal connection handling.
		onListenerError = p.instrument.WrapConnErrorHandler("proxy_lampshade_listen", onListenerError)
		if err := addListenerIfNecessary("lampshade", addrs.lampshade, p.listenLampshade(true, onListenerError, baseListen)); err != nil {
			return err
		}

		if err := addListenerIfNecessary("https", addrs.http, p.wrapTLSIfNecessary(p.listenHTTP(baseListen))); err != nil {
			return err
		}
		if err := addListenerIfNecessary("https_multiplex", addrs.httpMultiplex, p.wrapMultiplexing(p.wrapTLSIfNecessary(p.listenHTTP(baseListen)))); err != nil {
			return err
		}

		if err := addListenerIfNecessary("tlsmasq", addrs.tlsmasq, p.wrapMultiplexing(p.listenTLSMasq(baseListen))); err != nil {
			return err
		}

		return nil
	}

	if err := addListenerIfNecessary("kcp", p.KCPConf, p.wrapTLSIfNecessary(p.listenKCP)); err != nil {
		return err
	}
	if err := addListenerIfNecessary("quic_ietf", p.QUICIETFAddr, p.listenQUICIETF); err != nil {
		return err
	}
	if err := addListenerIfNecessary("shadowsocks", p.ShadowsocksAddr, p.listenShadowsocks); err != nil {
		return err
	}
	if err := addListenerIfNecessary("shadowsocks_multiplex", p.ShadowsocksMultiplexAddr, p.wrapMultiplexing(p.listenShadowsocks)); err != nil {
		return err
	}
	if err := addListenerIfNecessary("wss", p.WSSAddr, p.listenWSS); err != nil {
		return err
	}

	if err := addListenersForBaseTransport(p.listenTCP, &addresses{
		obfs4:          p.Obfs4Addr,
		obfs4Multiplex: p.Obfs4MultiplexAddr,
		lampshade:      p.LampshadeAddr,
		http:           p.HTTPAddr,
		httpMultiplex:  p.HTTPMultiplexAddr,
		tlsmasq:        p.TLSMasqAddr,
	}); err != nil {
		return err
	}

	errCh := make(chan error, len(allListeners))
	if p.EnableMultipath {
		mpl := multipath.NewListener(allListeners, p.instrument.MultipathStats(listenerProtocols))
		log.Debug("Serving multipath at:")
		for i, l := range allListeners {
			log.Debugf("  %-20s:  %v", listenerProtocols[i], l.Addr())
		}
		go func() {
			errCh <- srv.Serve(mpl, nil)
		}()
	} else {
		for _, _l := range allListeners {
			l := _l
			go func() {
				log.Debugf("Serving at: %v", l.Addr())
				errCh <- srv.Serve(l, mimic.SetServerAddr)
			}()
		}
	}
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// this is an expected path for closing, no error
		return err
	}
}

func (p *Proxy) ListenAndServeENHTTP() error {
	el, err := net.Listen("tcp", p.ENHTTPAddr)
	if err != nil {
		return errors.New("Unable to listen for encapsulated HTTP at %v: %v", p.ENHTTPAddr, err)
	}
	log.Debugf("Listening for encapsulated HTTP at %v", el.Addr())
	filterChain := filters.Join(tokenfilter.New(p.Token, p.instrument), p.instrument.WrapFilter("proxy_http_ping", ping.New(0)))
	enhttpHandler := enhttp.NewServerHandler(p.ENHTTPReapIdleTime, p.ENHTTPServerURL)
	server := &http.Server{
		Handler: filters.Intercept(enhttpHandler, p.instrument.WrapFilter("proxy", filterChain)),
	}
	return server.Serve(el)
}

func (p *Proxy) wrapTLSIfNecessary(fn listenerBuilderFN) listenerBuilderFN {
	return func(addr string) (net.Listener, error) {
		l, err := fn(addr)
		if err != nil {
			return nil, err
		}

		if p.HTTPS {
			l, err = tlslistener.Wrap(
				l, p.KeyFile, p.CertFile, p.SessionTicketKeyFile, p.FirstSessionTicketKey,
				p.RequireSessionTickets, p.MissingTicketReaction, p.TLSListenerAllowTLS13,
				p.instrument)
			if err != nil {
				return nil, err
			}

			log.Debugf("Using TLS on %v", l.Addr())
		}

		return l, nil
	}
}

func (p *Proxy) wrapMultiplexing(fn listenerBuilderFN) listenerBuilderFN {
	return func(addr string) (net.Listener, error) {
		l, err := fn(addr)
		if err != nil {
			return nil, err
		}

		var proto cmux.Protocol
		// smux is the default, but can be explicitly specified also
		if p.MultiplexProtocol == "" || p.MultiplexProtocol == "smux" {
			proto, err = p.buildSmuxProtocol()
		} else if p.MultiplexProtocol == "psmux" {
			proto, err = p.buildPsmuxProtocol()
		} else {
			err = errors.New("unknown multiplex protocol: %v", p.MultiplexProtocol)
		}
		if err != nil {
			return nil, err
		}

		l = cmux.Listen(&cmux.ListenOpts{
			Listener: l,
			Protocol: proto,
		})

		log.Debugf("Multiplexing on %v", l.Addr())
		return l, nil
	}
}

func (p *Proxy) buildSmuxProtocol() (cmux.Protocol, error) {
	config := smux.DefaultConfig()
	if p.SmuxVersion > 0 {
		config.Version = p.SmuxVersion
	}
	if p.SmuxMaxFrameSize > 0 {
		config.MaxFrameSize = p.SmuxMaxFrameSize
	}
	if p.SmuxMaxReceiveBuffer > 0 {
		config.MaxReceiveBuffer = p.SmuxMaxReceiveBuffer
	}
	if p.SmuxMaxStreamBuffer > 0 {
		config.MaxStreamBuffer = p.SmuxMaxStreamBuffer
	}
	return cmux.NewSmuxProtocol(config), nil
}

func (p *Proxy) buildPsmuxProtocol() (cmux.Protocol, error) {
	config := psmux.DefaultConfig()
	if p.PsmuxVersion > 0 {
		config.Version = p.PsmuxVersion
	}
	if p.PsmuxMaxFrameSize > 0 {
		config.MaxFrameSize = p.PsmuxMaxFrameSize
	}
	if p.PsmuxMaxReceiveBuffer > 0 {
		config.MaxReceiveBuffer = p.PsmuxMaxReceiveBuffer
	}
	if p.PsmuxMaxStreamBuffer > 0 {
		config.MaxStreamBuffer = p.PsmuxMaxStreamBuffer
	}
	if p.PsmuxDisablePadding {
		config.MaxPaddingRatio = 0.0
		config.MaxPaddedSize = 0
		config.AggressivePadding = 0
		config.AggressivePaddingRatio = 0.0
	} else {
		if p.PsmuxMaxPaddingRatio > 0.0 {
			config.MaxPaddingRatio = p.PsmuxMaxPaddingRatio
		}
		if p.PsmuxMaxPaddedSize > 0 {
			config.MaxPaddedSize = p.PsmuxMaxPaddedSize
		}
		if p.PsmuxDisableAggressivePadding {
			config.AggressivePadding = 0
			config.AggressivePaddingRatio = 0.0
		} else {
			if p.PsmuxAggressivePadding > 0 {
				config.AggressivePadding = p.PsmuxAggressivePadding
			}
			if p.PsmuxAggressivePaddingRatio > 0.0 {
				config.AggressivePaddingRatio = p.PsmuxAggressivePaddingRatio
			}
		}
	}
	return cmuxprivate.NewPsmuxProtocol(config), nil
}

func proxyNameAndDC(hostname string) (proxyName string, dc string) {
	match := proxyNameRegex.FindStringSubmatch(hostname)
	if len(match) != 5 {
		return proxyName, ""
	}
	return match[1], match[3]
}

func (p *Proxy) setBenchmarkMode() {
	if p.Benchmark {
		log.Debug("Putting proxy into benchmarking mode. Only a limited rate of requests to a specific set of domains will be allowed, no authentication token required.")
		p.HTTPS = true
		p.Token = "bench"
	}
}

func (p *Proxy) createBlacklist() *blacklist.Blacklist {
	return blacklist.New(blacklist.Options{
		MaxIdleTime:        p.BlacklistMaxIdleTime,        // 30 * time.Second,
		MaxConnectInterval: p.BlacklistMaxConnectInterval, // 5 * time.Second,
		AllowedFailures:    p.BlacklistAllowedFailures,    // 10,
		Expiration:         p.BlacklistExpiration,         // 6 * time.Hour,
	})
}

// createFilterChain creates a chain of filters that modify the default behavior
// of proxy.Proxy to implement Lantern-specific logic like authentication,
// Apache mimicry, bandwidth throttling, BBR metric reporting, etc. The actual
// work of proxying plain HTTP and CONNECT requests is handled by proxy.Proxy
// itself.
func (p *Proxy) createFilterChain(bl *blacklist.Blacklist) (filters.Chain, proxy.DialFunc, error) {
	filterChain := filters.Join(p.bm)

	if p.Benchmark {
		filterChain = filterChain.Append(proxyfilters.RateLimit(5000, map[string]time.Duration{
			"www.google.com":      30 * time.Minute,
			"www.facebook.com":    30 * time.Minute,
			"67.media.tumblr.com": 30 * time.Minute,
			"i.ytimg.com":         30 * time.Minute,    // YouTube play button
			"149.154.167.91":      30 * time.Minute,    // Telegram
			"ping-chained-server": 1 * time.Nanosecond, // Internal ping-chained-server protocol
		}))
	} else {
		filterChain = filterChain.Append(proxy.OnFirstOnly(tokenfilter.New(p.Token, p.instrument)))
	}

	if p.ReportingRedisClient == nil {
		log.Debug("Not enabling bandwidth limiting")
	} else {
		filterChain = filterChain.Append(
			proxy.OnFirstOnly(devicefilter.NewPre(
				redis.NewDeviceFetcher(p.ReportingRedisClient), p.throttleConfig, !p.Pro, p.instrument)),
		)
	}

	filterChain = filterChain.Append(
		proxy.OnFirstOnly(googlefilter.New(p.GoogleSearchRegex, p.GoogleCaptchaRegex)),
	)
	if p.ProxiedSitesSamplePercentage > 0 && p.ProxiedSitesTrackingID != "" {
		log.Debugf("Tracking proxied sites in Google Analytics")
		filterChain = filterChain.Append(analytics.New(&analytics.Options{
			TrackingID:       p.ProxiedSitesTrackingID,
			SamplePercentage: p.ProxiedSitesSamplePercentage,
		}),
		)
	} else {
		log.Debugf("Not tracking proxied sites in Google Analytics")
	}
	filterChain = filterChain.Append(proxy.OnFirstOnly(devicefilter.NewPost(bl)))

	if !p.TestingLocal {
		allowedLocalAddrs := []string{"127.0.0.1:7300"}
		if p.PacketForwardAddr != "" {
			allowedLocalAddrs = append(allowedLocalAddrs, p.PacketForwardAddr)
		}
		filterChain = filterChain.Append(proxyfilters.BlockLocal(allowedLocalAddrs))
	}
	filterChain = filterChain.Append(p.instrument.WrapFilter("proxy_http_ping", ping.New(0)))

	// Google anomaly detection can be triggered very often over IPv6.
	// Prefer IPv4 to mitigate, see issue #97
	_dialer := preferIPV4Dialer(timeoutToDialOriginSite)
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// resolve separately so that we can track the DNS resolution time
		resolvedAddr, resolveErr := net.ResolveTCPAddr(network, addr)
		if resolveErr != nil {
			return nil, resolveErr
		}

		conn, dialErr := _dialer(ctx, network, resolvedAddr.String())
		if dialErr != nil {
			return nil, dialErr
		}

		return conn, nil
	}
	dialerForPforward := dialer

	// Check if Lantern client version is in the supplied range. If yes,
	// redirect certain percentage of the requests to an URL to notify the user
	// to upgrade.
	if p.VersionCheck {
		log.Debugf("versioncheck: Will redirect %.4f%% of requests from Lantern clients %s to %s",
			p.VersionCheckRedirectPercentage*100,
			p.VersionCheckRange,
			p.VersionCheckRedirectURL,
		)
		vc, err := versioncheck.New(p.VersionCheckRange,
			p.VersionCheckRedirectURL,
			[]string{"80"}, // checks CONNECT tunnel to 80 port only.
			p.VersionCheckRedirectPercentage, p.instrument)
		if err != nil {
			log.Errorf("Fail to init versioncheck, skipping: %v", err)
		} else {
			dialerForPforward = vc.Dialer(dialerForPforward)
			filterChain = filterChain.Append(vc.Filter())
		}
	}

	filterChain = filterChain.Append(
		proxyfilters.DiscardInitialPersistentRequest,
		filters.FilterFunc(func(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
			if domains.ConfigForRequest(req).AddForwardedFor {
				// Only add X-Forwarded-For for certain domains
				return proxyfilters.AddForwardedFor(cs, req, next)
			}
			return next(cs, req)
		}),
		httpsupgrade.NewHTTPSUpgrade(p.CfgSvrAuthToken),
		proxyfilters.RestrictConnectPorts(p.allowedTunnelPorts()),
		proxyfilters.RecordOp,
		cleanheadersfilter.New(), // IMPORTANT, this should be the last filter in the chain to avoid stripping any headers that other filters might need
	)

	return filterChain, func(ctx context.Context, isCONNECT bool, network, addr string) (net.Conn, error) {
		if isCONNECT {
			return dialer(ctx, network, addr)
		}
		return dialerForPforward(ctx, network, addr)
	}, nil
}

func (p *Proxy) configureHoneycomb() func() {
	if p.HoneycombSampleRate <= 0 {
		log.Debug("Not configuring Honeycomb")
		return func() {}
	}

	log.Debug("Configuring Honeycomb")
	return p.configureOTEL(
		"api.honeycomb.io:443",
		map[string]string{
			"x-honeycomb-team": "jskJrfYyNNp2lcJ0WQ8JfD",
		},
		p.HoneycombSampleRate,
		1*time.Minute,
		false,
	)
}

func (p *Proxy) configureTeleport() func() {
	if p.TeleportSampleRate <= 0 {
		log.Debug("Not configuring Teleport")
		return func() {}
	}

	log.Debug("Configuring Teleport")
	return p.configureOTEL(
		"telemetry.iantem.io:443",
		map[string]string{},
		p.TeleportSampleRate,
		1*time.Hour,
		true,
	)
}

func (p *Proxy) configureOTEL(
	endpoint string,
	headers map[string]string,
	sampleRate int,
	reportingInterval time.Duration,
	includeDeviceIDs bool,
) func() {
	proxyName, dc := proxyNameAndDC(p.ProxyName)
	opts := &otel.Opts{
		Endpoint:      endpoint,
		Headers:       headers,
		SampleRate:    sampleRate,
		ExternalIP:    p.ExternalIP,
		ProxyName:     proxyName,
		Track:         p.Track,
		DC:            dc,
		ProxyProtocol: p.ProxyProtocol,
		IsPro:         p.Pro,
	}
	tp, stop := otel.BuildTracerProvider(opts)
	if tp != nil {
		go p.instrument.ReportToOTELPeriodically(reportingInterval, tp, includeDeviceIDs)
		ogStop := stop
		stop = func() {
			p.instrument.ReportToOTEL(tp, includeDeviceIDs)
			ogStop()
		}
	}
	return stop
}

func (p *Proxy) configureBandwidthReporting() *reportingConfig {
	return newReportingConfig(p.CountryLookup, p.ReportingRedisClient, p.EnableReports, p.instrument, p.throttleConfig)
}

func (p *Proxy) loadThrottleConfig() {
	if !p.Pro && p.ThrottleRefreshInterval > 0 && p.ReportingRedisClient != nil {
		p.throttleConfig = throttle.NewRedisConfig(p.ReportingRedisClient, p.ThrottleRefreshInterval)
	} else {
		log.Debug("Not loading throttle config")
		return
	}
}

func (p *Proxy) allowedTunnelPorts() []int {
	if p.TunnelPorts == "" {
		log.Debug("tunnelling all ports")
		return nil
	}
	ports, err := portsFromCSV(p.TunnelPorts)
	if err != nil {
		log.Fatal(err)
	}
	return ports
}

func (p *Proxy) listenHTTP(baseListen func(string, bool) (net.Listener, error)) listenerBuilderFN {
	return func(addr string) (net.Listener, error) {
		l, err := baseListen(addr, true)
		if err != nil {
			return nil, errors.New("Unable to listen for HTTP: %v", err)
		}
		log.Debugf("Listening for HTTP(S) at %v", l.Addr())
		return l, nil
	}
}

func (p *Proxy) listenOBFS4(baseListen func(string, bool) (net.Listener, error)) listenerBuilderFN {
	return func(addr string) (net.Listener, error) {
		l, err := baseListen(addr, true)
		if err != nil {
			return nil, errors.New("Unable to listen for OBFS4: %v", err)
		}
		wrapped, err := obfs4listener.Wrap(l, p.Obfs4Dir, p.Obfs4HandshakeConcurrency, p.Obfs4MaxPendingHandshakesPerClient, p.Obfs4HandshakeTimeout)
		if err != nil {
			l.Close()
			return nil, errors.New("Unable to wrap listener with OBFS4: %v", err)
		}
		log.Debugf("Listening for OBFS4 at %v", wrapped.Addr())
		return wrapped, nil
	}
}

func (p *Proxy) listenLampshade(trackBBR bool, onListenerError func(net.Conn, error), baseListen func(string, bool) (net.Listener, error)) listenerBuilderFN {
	return func(addr string) (net.Listener, error) {
		l, err := baseListen(addr, false)
		if err != nil {
			return nil, err
		}
		wrapped, wrapErr := lampshade.Wrap(l, p.CertFile, p.KeyFile, p.LampshadeKeyCacheSize, p.LampshadeMaxClientInitAge, onListenerError)
		if wrapErr != nil {
			log.Fatalf("Unable to initialize lampshade with tcp: %v", wrapErr)
		}
		log.Debugf("Listening for lampshade at %v", wrapped.Addr())

		if trackBBR {
			// We wrap the lampshade listener itself so that we record BBR metrics on
			// close of virtual streams rather than the physical connection.
			wrapped = p.bm.Wrap(wrapped)
		}

		// Wrap lampshade streams with idletiming as well
		wrapped = listeners.NewIdleConnListener(wrapped, p.IdleTimeout)

		return wrapped, nil
	}
}

func (p *Proxy) listenTLSMasq(baseListen func(string, bool) (net.Listener, error)) listenerBuilderFN {
	return func(addr string) (net.Listener, error) {
		l, err := baseListen(addr, false)
		if err != nil {
			return nil, err
		}

		nonFatalErrorsHandler := func(err error) {
			log.Debugf("non-fatal error from tlsmasq: %v", err)
		}

		wrapped, wrapErr := tlsmasq.Wrap(
			l, p.CertFile, p.KeyFile, p.TLSMasqOriginAddr, p.TLSMasqSecret,
			p.TLSMasqTLSMinVersion, p.TLSMasqTLSCipherSuites, nonFatalErrorsHandler)
		if wrapErr != nil {
			log.Fatalf("unable to wrap listener with tlsmasq: %v", wrapErr)
		}
		log.Debugf("listening for tlsmasq at %v", wrapped.Addr())

		return wrapped, nil
	}
}

func (p *Proxy) listenTCP(addr string, wrapBBR bool) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if p.IdleTimeout > 0 {
		l = listeners.NewIdleConnListener(l, p.IdleTimeout)
	}
	if p.DiffServTOS > 0 {
		log.Debugf("Setting diffserv TOS to %d", p.DiffServTOS)
		// Note - this doesn't actually wrap the underlying connection, it'll still
		// be a net.TCPConn
		l = diffserv.Wrap(l, p.DiffServTOS)
	} else {
		log.Debugf("Not setting diffserv TOS")
	}
	if wrapBBR {
		l = p.bm.Wrap(l)
	}
	return l, nil
}

func (p *Proxy) listenKCP(kcpConf string) (net.Listener, error) {
	cfg := &kcpwrapper.ListenerConfig{}
	file, err := os.Open(kcpConf) // For read access.
	if err != nil {
		return nil, errors.New("Unable to open KCPConf at %v: %v", p.KCPConf, err)
	}

	err = json.NewDecoder(file).Decode(cfg)
	file.Close()
	if err != nil {
		return nil, errors.New("Unable to decode KCPConf at %v: %v", p.KCPConf, err)
	}

	log.Debugf("Listening KCP at %v", cfg.Listen)
	return kcpwrapper.Listen(cfg, func(conn net.Conn) net.Conn {
		if p.IdleTimeout <= 0 {
			return conn
		}
		return listeners.WrapIdleConn(conn, p.IdleTimeout)
	})
}

func (p *Proxy) listenQUICIETF(addr string) (net.Listener, error) {
	tlsConf, err := tlsdefaults.BuildListenerConfig(addr, p.KeyFile, p.CertFile)
	if err != nil {
		return nil, err
	}

	config := &quicwrapper.Config{
		MaxIncomingStreams:      1000,
		Tracer:                  instrument.NewQuicTracer(p.instrument),
		UseBBR:                  p.QUICUseBBR,
		DisablePathMTUDiscovery: true,
	}

	l, err := quicwrapper.ListenAddr(p.QUICIETFAddr, tlsConf, config)
	if err != nil {
		return nil, err
	}

	log.Debugf("Listening for quic at %v", l.Addr())
	return l, err
}

func (p *Proxy) listenShadowsocks(addr string) (net.Listener, error) {
	// This is not using p.ListenTCP on purpose to avoid additional wrapping with idle timing.
	// The idea here is to be as close to what outline shadowsocks does without any intervention,
	// especially with respect to draining connections and the timing of closures.

	configs := []shadowsocks.CipherConfig{
		shadowsocks.CipherConfig{
			ID:     "default",
			Secret: p.ShadowsocksSecret,
			Cipher: p.ShadowsocksCipher,
		},
	}
	ciphers, err := shadowsocks.NewCipherListWithConfigs(configs)
	if err != nil {
		return nil, errors.New("Unable to create shadowsocks cipher: %v", err)
	}
	l, err := shadowsocks.ListenLocalTCP(
		addr, ciphers,
		p.ShadowsocksReplayHistory,
	)
	if err != nil {
		return nil, errors.New("Unable to listen for shadowsocks: %v", err)
	}

	log.Debugf("Listening for shadowsocks at %v", l.Addr())
	return l, nil
}

func (p *Proxy) listenWSS(addr string) (net.Listener, error) {
	l, err := p.listenTCP(addr, true)
	if err != nil {
		return nil, errors.New("Unable to listen for wss: %v", err)
	}

	if p.HTTPS {
		l, err = tlslistener.Wrap(
			l, p.KeyFile, p.CertFile, p.SessionTicketKeyFile, p.FirstSessionTicketKey,
			p.RequireSessionTickets, p.MissingTicketReaction, p.TLSListenerAllowTLS13, p.instrument)
		if err != nil {
			return nil, err
		}
		log.Debugf("Using TLS on %v", l.Addr())
	}
	opts := &tinywss.ListenOpts{
		Listener: l,
	}

	l, err = tinywss.ListenAddr(opts)
	if err != nil {
		return nil, err
	}

	log.Debugf("Listening for wss at %v", l.Addr())
	return l, err
}

func (p *Proxy) setupPacketForward() error {
	if runtime.GOOS != "linux" {
		log.Debugf("Ignoring packet forward on %v", runtime.GOOS)
		return nil
	}
	if p.PacketForwardAddr == "" {
		return nil
	}
	l, err := net.Listen("tcp", p.PacketForwardAddr)
	if err != nil {
		return errors.New("Unable to listen for packet forwarding at %v: %v", p.PacketForwardAddr, err)
	}
	s, err := packetforward.NewServer(&packetforward.Opts{
		Opts: gonat.Opts{
			StatsInterval: 15 * time.Second,
			IFName:        p.ExternalIntf,
			IdleTimeout:   90 * time.Second,
			BufferDepth:   1000,
		},
		BufferPoolSize: 50 * 1024 * 1024,
	})
	if err != nil {
		return errors.New("Error configuring packet forwarding: %v", err)
	}
	log.Debugf("Listening for packet forwarding at %v", l.Addr())

	go func() {
		if err := s.Serve(l); err != nil {
			log.Errorf("Error serving packet forwarding: %v", err)
		}
	}()
	return nil
}

func portsFromCSV(csv string) ([]int, error) {
	fields := strings.Split(csv, ",")
	ports := make([]int, len(fields))
	for i, f := range fields {
		p, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, err
		}
		ports[i] = p
	}
	return ports, nil
}
