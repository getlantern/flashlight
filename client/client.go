package client

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru"

	"github.com/getlantern/detour"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/go-socks5"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/httpseverywhere"
	"github.com/getlantern/i18n"
	"github.com/getlantern/iptool"
	"github.com/getlantern/mitm"
	"github.com/getlantern/netx"
	"github.com/getlantern/proxy"
	"github.com/getlantern/proxy/filters"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/status"
)

var (
	log = golog.LoggerFor("flashlight.client")

	translationAppName = strings.ToUpper(common.AppName)

	addr                = eventual.NewValue()
	socksAddr           = eventual.NewValue()
	proxiedCONNECTPorts = []int{
		// Standard HTTP(S) ports
		80, 443,
		// SSH for those who know how to configure it
		22,
		// SMTP and encrypted SMTP
		25, 465,
		// POP and encrypted POP
		110, 995,
		// IMAP and encrypted IMAP
		143, 993,
		// Common unprivileged HTTP(S) ports
		8080, 8443,
		// XMPP
		5222, 5223, 5224,
		// Android
		5228, 5229,
		// udpgw
		7300,
		// Google Hangouts/Meet TCP Ports (see https://support.google.com/a/answer/7582935#ports)
		19302, 19303, 19304, 19305, 19306, 19307, 19308, 19309,
	}

	// Set a hard limit when processing proxy requests. Should be short enough to
	// avoid applications bypassing Lantern.
	// Chrome has a 30s timeout before marking proxy as bad.
	// Also used as the overall dial timeout for the balancer.
	requestTimeout = 20 * time.Second

	// interval before rewriting the same URL to HTTPS, to avoid redirect loop.
	httpsRewriteInterval = 10 * time.Second

	forever = time.Duration(math.MaxInt64)

	// See http://stackoverflow.com/questions/106179/regular-expression-to-match-dns-hostname-or-ip-address
	validHostnameRegex = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)

	errLanternOff = fmt.Errorf("Lantern is off")

	forceProxying int64
)

// ForceProxying forces everything to get proxied (useful for testing)
func ForceProxying() {
	atomic.StoreInt64(&forceProxying, 1)
}

// StopForcingProxying disables forced proxying (useful for testing)
func StopForcingProxying() {
	atomic.StoreInt64(&forceProxying, 0)
}

func shouldForceProxying() bool {
	return atomic.LoadInt64(&forceProxying) == 1
}

// Client is an HTTP proxy that accepts connections from local programs and
// proxies these via remote flashlight servers.
type Client struct {
	configDir string

	// requestTimeout: (optional) timeout to process the request from application
	requestTimeout time.Duration

	// Balanced CONNECT dialers.
	bal *balancer.Balancer

	proxy proxy.Proxy

	l net.Listener

	disconnected         func() bool
	allowProbes          func() bool
	proxyAll             func() bool
	useShortcut          func() bool
	allowShortcutTo      func(ctx context.Context, addr string) (bool, net.IP)
	useDetour            func() bool
	allowHTTPSEverywhere func() bool
	user                 common.UserConfig

	rewriteToHTTPS httpseverywhere.Rewrite
	rewriteLRU     *lru.Cache

	statsTracker stats.Tracker

	iptool            iptool.Tool
	allowPrivateHosts func() bool
	lang              func() string
	adSwapTargetURL   func() string

	reverseDNS func(addr string) string

	httpProxyIP   string
	httpProxyPort string

	chPingProxiesConf chan pingProxiesConf
}

// NewClient creates a new client that does things like starts the HTTP and
// SOCKS proxies. It take a function for determing whether or not to proxy
// all traffic, and another function to get Lantern Pro token when required.
func NewClient(
	configDir string,
	disconnected func() bool,
	allowProbes func() bool,
	proxyAll func() bool,
	useShortcut func() bool,
	allowShortcutTo func(ctx context.Context, addr string) (bool, net.IP),
	useDetour func() bool,
	allowHTTPSEverywhere func() bool,
	allowMITM func() bool,
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	allowPrivateHosts func() bool,
	lang func() string,
	adSwapTargetURL func() string,
	reverseDNS func(addr string) string,
) (*Client, error) {
	// A small LRU to detect redirect loop
	rewriteLRU, err := lru.New(100)
	if err != nil {
		return nil, errors.New("Unable to create rewrite LRU: %v", err)
	}
	client := &Client{
		configDir:            configDir,
		requestTimeout:       requestTimeout,
		bal:                  balancer.New(allowProbes, time.Duration(requestTimeout)),
		disconnected:         disconnected,
		allowProbes:          allowProbes,
		proxyAll:             proxyAll,
		useShortcut:          useShortcut,
		allowShortcutTo:      allowShortcutTo,
		useDetour:            useDetour,
		allowHTTPSEverywhere: allowHTTPSEverywhere,
		user:                 userConfig,
		rewriteToHTTPS:       httpseverywhere.Default(),
		rewriteLRU:           rewriteLRU,
		statsTracker:         statsTracker,
		allowPrivateHosts:    allowPrivateHosts,
		lang:                 lang,
		adSwapTargetURL:      adSwapTargetURL,
		reverseDNS:           reverseDNS,
		chPingProxiesConf:    make(chan pingProxiesConf, 1),
	}

	keepAliveIdleTimeout := chained.IdleTimeout - 5*time.Second

	var mitmOpts *mitm.Opts
	if allowMITM() {
		log.Debug("Enabling MITM")
		mitmOpts = &mitm.Opts{
			PKFile:             filepath.Join(configDir, "mitmkey.pem"),
			CertFile:           filepath.Join(configDir, "mitmcert.pem"),
			Organization:       "Lantern",
			InstallCert:        true,
			InstallPrompt:      i18n.T("BACKEND_MITM_INSTALL_CERT", i18n.T(translationAppName)),
			WindowsPromptTitle: i18n.T("BACKEND_MITM_INSTALL_CERT", i18n.T(translationAppName)),
			WindowsPromptBody:  i18n.T("BACKEND_MITM_INSTALL_CERT_WINDOWS_BODY", "certimporter.exe"),
			InstallCertResult: func(installErr error) {
				op := ops.Begin("install_mitm_cert")
				op.FailIf(installErr)
				if installErr == nil {
					log.Debug("Successfully installed MITM cert")
				}
				op.End()
			},
			Domains: []string{
				// Currently don't bother MITM'ing ad sites since we're not doing ad swapping
				// "*.doubleclick.net",
				// "*.g.doubleclick.net",
				// "adservice.google.com",
				// "adservice.google.com.hk",
				// "adservice.google.co.jp",
				// "adservice.google.nl",
				// "*.googlesyndication.com",
				// "*.googletagservices.com",
				// "googleadservices.com",
				// MITM YouTube domains to track statistics on watched videos (list obtained by running ../cmd/youtubescanner/youtubescanner.go)
				"*.youtube.ae",
				"*.youtube.co.ae",
				"*.youtube.com.ar",
				"*.youtube.at",
				"*.youtube.co.at",
				"*.youtube.com.au",
				"*.youtube.az",
				"*.youtube.com.az",
				"*.youtube.ba",
				"*.youtube.be",
				"*.youtube.bg",
				"*.youtube.bh",
				"*.youtube.com.bh",
				"*.youtube.bo",
				"*.youtube.com.bo",
				"*.youtube.com.br",
				"*.youtube.com.by",
				"*.youtube.by",
				"*.youtube.ca",
				"*.youtube.cat",
				"*.youtube.ch",
				"*.youtube.cl",
				"*.youtube.co",
				"*.youtube.com.co",
				"*.youtube.com",
				"*.youtube.cr",
				"*.youtube.co.cr",
				"*.youtube.cz",
				"*.youtube.de",
				"*.youtube.dk",
				"*.youtube.com.do",
				"*.youtube.com.ec",
				"*.youtube.ee",
				"*.youtube.com.ee",
				"*.youtube.com.eg",
				"*.youtube.es",
				"*.youtube.com.es",
				"*.youtube.fi",
				"*.youtube.fr",
				"*.youtube.ge",
				"*.youtube.com.gh",
				"*.youtube.gr",
				"*.youtube.com.gr",
				"*.youtube.gt",
				"*.youtube.com.gt",
				"*.youtube.hk",
				"*.youtube.com.hk",
				"*.youtube.com.hn",
				"*.youtube.hr",
				"*.youtube.com.hr",
				"*.youtube.hu",
				"*.youtube.co.hu",
				"*.youtube.co.id",
				"*.youtube.ie",
				"*.youtube.co.il",
				"*.youtube.in",
				"*.youtube.co.in",
				"*.youtube.iq",
				"*.youtube.is",
				"*.youtube.it",
				"*.youtube.com.jm",
				"*.youtube.com.jo",
				"*.youtube.jo",
				"*.youtube.jp",
				"*.youtube.co.jp",
				"*.youtube.co.ke",
				"*.youtube.kr",
				"*.youtube.co.kr",
				"*.youtube.com.kw",
				"*.youtube.kz",
				"*.youtube.com.lb",
				"*.youtube.lk",
				"*.youtube.lt",
				"*.youtube.lu",
				"*.youtube.lv",
				"*.youtube.com.lv",
				"*.youtube.ly",
				"*.youtube.com.ly",
				"*.youtube.co.ma",
				"*.youtube.ma",
				"*.youtube.me",
				"*.youtube.com.mk",
				"*.youtube.mk",
				"*.youtube.com.mt",
				"*.youtube.mx",
				"*.youtube.com.mx",
				"*.youtube.my",
				"*.youtube.com.my",
				"*.youtube.ng",
				"*.youtube.com.ng",
				"*.youtube.ni",
				"*.youtube.com.ni",
				"*.youtube.nl",
				"*.youtube.no",
				"*.youtube.co.nz",
				"*.youtube.com.om",
				"*.youtube.pa",
				"*.youtube.com.pa",
				"*.youtube.pe",
				"*.youtube.com.pe",
				"*.youtube.ph",
				"*.youtube.com.ph",
				"*.youtube.pk",
				"*.youtube.com.pk",
				"*.youtube.pl",
				"*.youtube.pr",
				"*.youtube.pt",
				"*.youtube.com.pt",
				"*.youtube.qa",
				"*.youtube.com.py",
				"*.youtube.com.qa",
				"*.youtube.ro",
				"*.youtube.com.ro",
				"*.youtube.rs",
				"*.youtube.ru",
				"*.youtube.sa",
				"*.youtube.com.sa",
				"*.youtube.se",
				"*.youtube.sg",
				"*.youtube.com.sg",
				"*.youtube.si",
				"*.youtube.sk",
				"*.youtube.sn",
				"*.youtube.sv",
				"*.youtube.com.sv",
				"*.youtube.co.th",
				"*.youtube.tn",
				"*.youtube.com.tn",
				"*.youtube.com.tr",
				"*.youtube.com.tw",
				"*.youtube.co.tz",
				"*.youtube.ua",
				"*.youtube.com.ua",
				"*.youtube.ug",
				"*.youtube.co.ug",
				"*.youtube.co.uk",
				"*.youtube.com.uy",
				"*.youtube.uy",
				"*.youtube.com.ve",
				"*.youtube.co.ve",
				"*.youtube.vn",
				"*.youtube.voto",
				"*.youtube.co.za",
				"*.youtube.co.zw",
			},
		}
	}
	var mitmErr error
	client.proxy, mitmErr = proxy.New(&proxy.Opts{
		IdleTimeout:  keepAliveIdleTimeout,
		BufferSource: buffers.Pool,
		Filter:       filters.FilterFunc(client.filter),
		OnError:      errorResponse,
		Dial:         client.dial,
		MITMOpts:     mitmOpts,
		ShouldMITM: func(req *http.Request, upstreamAddr string) bool {
			userAgent := req.Header.Get("User-Agent")
			// Only MITM certain browsers
			// See http://useragentstring.com/pages/useragentstring.php
			shouldMITM := strings.Contains(userAgent, "Chrome/") || // Chrome
				strings.Contains(userAgent, "MSIE") || strings.Contains(userAgent, "Trident") || // Internet Explorer
				strings.Contains(userAgent, "Edge") || // Microsoft Edge
				strings.Contains(userAgent, "QQBrowser") || // QQ
				strings.Contains(userAgent, "360Browser") || strings.Contains(userAgent, "360SE") || strings.Contains(userAgent, "360EE") // 360
			return shouldMITM
		},
	})
	if mitmErr != nil {
		log.Errorf("Unable to initialize MITM: %v", mitmErr)
	}
	client.reportProxyLocationLoop()
	client.iptool, err = iptool.New()
	if err != nil {
		return nil, errors.New("Unable to initialize iptool: %v", err)
	}
	go client.pingProxiesLoop()
	return client, nil
}

func (client *Client) GetBalancer() *balancer.Balancer {
	return client.bal
}

func (client *Client) reportProxyLocationLoop() {
	ch := client.bal.OnActiveDialer()
	var activeProxy string
	go func() {
		for {
			proxy := <-ch
			if proxy.Name() == activeProxy {
				continue
			}
			countryCode, country, city := proxyLoc(proxy)
			client.statsTracker.SetActiveProxyLocation(
				city,
				country,
				countryCode,
			)
		}
	}()
}

// Addr returns the address at which the client is listening with HTTP, blocking
// until the given timeout for an address to become available.
func Addr(timeout time.Duration) (interface{}, bool) {
	return addr.Get(timeout)
}

// Addr returns the address at which the client is listening with HTTP, blocking
// until the given timeout for an address to become available.
func (client *Client) Addr(timeout time.Duration) (interface{}, bool) {
	return Addr(timeout)
}

// Socks5Addr returns the address at which the client is listening with SOCKS5,
// blocking until the given timeout for an address to become available.
func Socks5Addr(timeout time.Duration) (interface{}, bool) {
	return socksAddr.Get(timeout)
}

// Socks5Addr returns the address at which the client is listening with SOCKS5,
// blocking until the given timeout for an address to become available.
func (client *Client) Socks5Addr(timeout time.Duration) (interface{}, bool) {
	return Socks5Addr(timeout)
}

// ListenAndServeHTTP makes the client listen for HTTP connections at the given
// address or, if a blank address is given, at a random port on localhost.
// onListeningFn is a callback that gets invoked as soon as the server is
// accepting TCP connections.
// Sometimes on Windows, http.Server may fail to accept new connections after
// running for a random period. This method will try serve again.
func (client *Client) ListenAndServeHTTP(requestedAddr string, onListeningFn func()) error {
	var err error
	var l net.Listener
	log.Debugf("About to listen at '%s'", requestedAddr)
	if l, err = net.Listen("tcp", requestedAddr); err != nil {
		log.Debugf("Error listening at '%s', fallback to default: %v", requestedAddr, err)
		requestedAddr = "127.0.0.1:0"
		log.Debugf("About to listen at '%s'", requestedAddr)
		if l, err = net.Listen("tcp", requestedAddr); err != nil {
			return fmt.Errorf("Unable to listen: %q", err)
		}
	}

	client.l = l
	listenAddr := l.Addr().String()
	addr.Set(listenAddr)
	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort(listenAddr)
	onListeningFn()

	log.Debugf("About to start HTTP client proxy at %v", listenAddr)
	for {
		start := time.Now()
		conn, err := l.Accept()
		if err != nil {
			if time.Since(start) >= 10*time.Second {
				log.Debugf("Error serving HTTP client proxy at %v, restarting: %v", listenAddr, err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return fmt.Errorf("Unable to accept connection: %v", err)
		}
		go client.handle(conn)
	}
}

// ListenAndServeSOCKS5 starts the SOCKS server listening at the specified
// address.
func (client *Client) ListenAndServeSOCKS5(requestedAddr string) error {
	var err error
	var l net.Listener
	if l, err = net.Listen("tcp", requestedAddr); err != nil {
		return fmt.Errorf("Unable to listen: %q", err)
	}
	l = &optimisticListener{l}
	listenAddr := l.Addr().String()
	socksAddr.Set(listenAddr)

	conf := &socks5.Config{
		HandleConnect: func(ctx context.Context, conn net.Conn, req *socks5.Request, replySuccess func(boundAddr net.Addr) error, replyError func(err error) error) error {
			op, ctx := ops.BeginWithNewBeam("proxy", ctx)
			defer op.End()
			addr := client.reverseDNS(fmt.Sprintf("%v:%v", req.DestAddr.IP, req.DestAddr.Port))
			errOnReply := replySuccess(nil)
			if errOnReply != nil {
				return op.FailIf(log.Errorf("Unable to reply success to SOCKS5 client: %v", errOnReply))
			}
			return op.FailIf(client.proxy.Connect(ctx, req.BufConn, conn, addr))
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("Unable to create SOCKS5 server: %v", err)
	}

	log.Debugf("About to start SOCKS5 client proxy at %v", listenAddr)
	return server.Serve(l)
}

// Configure updates the client's configuration. Configure can be called
// before or after ListenAndServe, and can be called multiple times. If
// no error occurred, then the new dialers are returned.
func (client *Client) Configure(proxies map[string]*chained.ChainedServerInfo) []balancer.Dialer {
	log.Debug("Configure() called")
	dialers, err := client.initBalancer(proxies)
	if err != nil {
		log.Error(err)
		return nil
	}
	chained.PersistSessionStates(client.configDir)
	chained.TrackStatsFor(dialers, client.configDir, client.allowProbes())
	return dialers
}

// Stop is called when the client is no longer needed. It closes the
// client listener and underlying dialer connection pool
func (client *Client) Stop() error {
	return client.l.Close()
}

func (client *Client) dial(ctx context.Context, isConnect bool, network, addr string) (conn net.Conn, err error) {
	op := ops.BeginWithBeam("proxied_dialer", ctx)
	op.Set("local_proxy_type", "http")
	op.Origin(addr, "")
	defer op.End()
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	return client.doDial(op, ctx, isConnect, addr)
}

// doDial is the ultimate place to dial an origin site. It takes following steps:
// * If the addr is in the proxied sites list or previously detected by detour as blocked, dial the site through proxies.
// * If proxyAll is on, dial the site through proxies.
// * If the host or port is configured not proxyable, dial directly.
// * If the site is allowed by shortcut, dial directly. If it failed before the deadline, try proxying.
// * Try dial the site directly with 1/5th of the requestTimeout, then try proxying.
func (client *Client) doDial(op *ops.Op, ctx context.Context, isCONNECT bool, addr string) (net.Conn, error) {

	dialDirect := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return netx.DialContext(ctx, network, addr)
	}

	dialProxied := func(ctx context.Context, _unused, addr string) (net.Conn, error) {
		op.Set("remotely_proxied", true)
		proto := balancer.NetworkPersistent
		if isCONNECT {
			// UGLY HACK ALERT! In this case, we know we need to send a CONNECT request
			// to the chained server. We need to send that request from chained/dialer.go
			// though because only it knows about the authentication token to use.
			// We signal it to send the CONNECT here using the network transport argument
			// that is effectively always "tcp" in the end, but we look for this
			// special "transport" in the dialer and send a CONNECT request in that
			// case.
			proto = balancer.NetworkConnect
		}
		start := time.Now()
		conn, err := client.bal.DialContext(ctx, proto, addr)
		if log.IsTraceEnabled() {
			log.Tracef("Dialing proxy takes %v for %s", time.Since(start), addr)
		}
		if conn != nil {
			conn = &proxiedConn{conn}
		}
		return conn, err
	}

	if shouldForceProxying() {
		log.Tracef("Proxying to %v because everything is forced to be proxied", addr)
		op.Set("force_proxied", true)
		op.Set("force_proxied_reason", "forceproxying")
		return dialProxied(ctx, "whatever", addr)
	}

	if err := client.allowSendingToProxy(addr); err != nil {
		log.Debugf("%v, sending directly to %v", err, addr)
		op.Set("force_direct", true)
		op.Set("force_direct_reason", err.Error())
		return dialDirect(ctx, "tcp", addr)
	}

	if client.proxyAll() {
		log.Tracef("Proxying to %v because proxyall is enabled", addr)
		op.Set("force_proxied", true)
		op.Set("force_proxied_reason", "proxyall")
		return dialProxied(ctx, "whatever", addr)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.ToLower(strings.TrimSpace(host))

	switch domainrouting.RuleFor(host) {
	case domainrouting.Direct:
		log.Tracef("Directly dialing %v per domain routing rules", addr)
		op.Set("force_direct", true)
		op.Set("force_direct_reason", "routingrule")
		return dialDirect(ctx, "tcp", addr)
	case domainrouting.Proxy:
		log.Tracef("Proxying to %v per domain routing rules", addr)
		op.Set("force_proxied", true)
		op.Set("force_proxied_reason", "routingrule")
		return dialProxied(ctx, "whatever", addr)
	}

	dialDirectForShortcut := func(ctx context.Context, network, addr string, ip net.IP) (net.Conn, error) {
		log.Debugf("Use shortcut (dial directly) for %v(%v)", addr, ip)
		op.Set("shortcut_direct", true)
		op.Set("shortcut_direct_ip", ip)
		return dialDirect(ctx, "tcp", addr)
	}

	dl, _ := ctx.Deadline()
	// It's roughly requestTimeout (20s) / 5 = 4s to leave enough time
	// to try dialing via proxies. Not hardcode to 4s to avoid break test
	// code which may have a shorter requestTimeout.
	directTimeout := dl.Sub(time.Now()) / 5
	cappedCTX, cancel := context.WithTimeout(ctx, directTimeout)
	defer cancel()

	dialDirectForDetour := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if client.useShortcut() {
			if allow, ip := client.allowShortcutTo(cappedCTX, addr); allow {
				// Arbitrarily have a larger timeout if the address is eligible for shortcut.
				shortcutCTX, cancel := context.WithTimeout(ctx, directTimeout*2)
				defer cancel()
				return dialDirectForShortcut(shortcutCTX, network, addr, ip)
			}
		}
		log.Tracef("Dialing %v directly for detour", addr)
		return dialDirect(cappedCTX, network, addr)
	}

	var dialer func(ctx context.Context, network, addr string) (net.Conn, error)
	if client.useDetour() {
		op.Set("detour", true)
		dialer = detour.Dialer(dialDirectForDetour, dialProxied)
	} else if !client.useShortcut() {
		dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Tracef("Dialing %v directly because neither detour nor shortcut is enabled", addr)
			return dialDirect(ctx, network, addr)
		}
	} else {
		dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var conn net.Conn
			var err error
			if allow, ip := client.allowShortcutTo(cappedCTX, addr); allow {
				// Don't cap the context if the address is eligible for shortcut.
				conn, err = dialDirectForShortcut(ctx, network, addr, ip)
				if err == nil {
					return conn, err
				}
			}
			select {
			case <-ctx.Done():
				if err == nil {
					err = ctx.Err()
				}
				return nil, err
			default:
				return dialProxied(ctx, network, addr)
			}
		}
	}
	return dialer(ctx, "tcp", addr)
}

func (client *Client) allowSendingToProxy(addr string) error {
	if client.disconnected() {
		return errLanternOff
	}

	port, err := client.portForAddress(addr)
	if err != nil {
		return err
	}

	if err := client.isPortProxyable(port); err != nil {
		return err
	}
	return client.isAddressProxyable(addr)
}

func (client *Client) isPortProxyable(port int) error {
	for _, proxiedPort := range proxiedCONNECTPorts {
		if port == proxiedPort {
			return nil
		}
	}
	return fmt.Errorf("Port %d not proxyable", port)
}

// isAddressProxyable largely replicates the logic in the old PAC file
func (client *Client) isAddressProxyable(addr string) error {
	if client.allowPrivateHosts() {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("Unable to split host and port for %v, considering private: %v", addr, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// host is not an IP address
		if validHostnameRegex.MatchString(host) {
			if !strings.Contains(host, ".") {
				return fmt.Errorf("%v is a plain hostname, considering private", host)
			}
			if strings.HasSuffix(host, ".local") {
				return fmt.Errorf("%v ends in .local, considering private", host)
			}
		}
		// assuming non-private
		return nil
	}

	ipAddrToCheck := &net.IPAddr{IP: ip}
	if client.iptool.IsPrivate(ipAddrToCheck) {
		return fmt.Errorf("IP %v is private", host)
	}
	return nil
}

func (client *Client) portForAddress(addr string) (int, error) {
	_, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("Unable to determine port for address %v: %v", addr, err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return 0, fmt.Errorf("Unable to parse port %v for address %v: %v", addr, port, err)
	}
	return port, nil
}

func errorResponse(ctx filters.Context, req *http.Request, read bool, err error) *http.Response {
	var htmlerr []byte

	if req == nil {
		return nil
	}

	// If the request has an 'Accept' header preferring HTML, or
	// doesn't have that header at all, render the error page.
	switch req.Header.Get("Accept") {
	case "text/html":
		fallthrough
	case "application/xhtml+xml":
		fallthrough
	case "":
		// It is likely we will have lots of different errors to handle but for now
		// we will only return a ErrorAccessingPage error.  This prevents the user
		// from getting just a blank screen.
		htmlerr, err = status.ErrorAccessingPage(req.Host, err)
		if err != nil {
			log.Debugf("Got error while generating status page: %q", err)
		}
	}

	if htmlerr == nil {
		// Default value for htmlerr
		htmlerr = []byte(hidden.Clean(err.Error()))
	}

	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBuffer(htmlerr)),
		StatusCode: http.StatusServiceUnavailable,
	}
}
