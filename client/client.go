package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/detour"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/go-socks5"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/httpseverywhere"
	"github.com/getlantern/iptool"
	"github.com/getlantern/netx"
	"github.com/getlantern/proxy/v3"
	"github.com/getlantern/proxy/v3/filters"
	"github.com/getlantern/shortcut"

	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/stats"
	"github.com/getlantern/flashlight/v7/status"
)

var (
	log = golog.LoggerFor("flashlight.client")

	addr                = eventual.NewValue()
	socksAddr           = eventual.NewValue()
	proxiedCONNECTPorts = []int{
		// Standard HTTP(S) ports
		80, 443,
		// SSH for those who know how to configure it
		22,
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

	// See http://stackoverflow.com/questions/106179/regular-expression-to-match-dns-hostname-or-ip-address
	validHostnameRegex = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)

	errLanternOff = fmt.Errorf("lantern is off")

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

	dialer *chained.Selector

	proxy proxy.Proxy

	httpListener  eventual.Value
	socksListener eventual.Value

	disconnected         func() bool
	proxyAll             func() bool
	useShortcut          func() bool
	shortcutMethod       func(ctx context.Context, addr string) (shortcut.Method, net.IP)
	useDetour            func() bool
	allowHTTPSEverywhere func() bool
	onDialError          func(error, bool)
	onSucceedingProxy    func()
	user                 common.UserConfig

	rewriteToHTTPS httpseverywhere.Rewrite
	rewriteLRU     *lru.Cache

	statsTracker stats.Tracker

	// There will be one op in the map per open connection.
	opsMap *opsMap

	iptool            iptool.Tool
	allowPrivateHosts func() bool
	lang              func() string
	adSwapTargetURL   func() string

	reverseDNS func(addr string) (string, error)

	httpProxyIP   string
	httpProxyPort string

	eventWithLabel func(category, action, label string)

	httpWg  sync.WaitGroup
	socksWg sync.WaitGroup

	DNSResolutionMapForDirectDialsEventual eventual.Value
}

// NewClient creates a new client that does things like starts the HTTP and
// SOCKS proxies. It take a function for determing whether or not to proxy
// all traffic, and another function to get Lantern Pro token when required.
func NewClient(
	configDir string,
	disconnected func() bool,
	proxyAll func() bool,
	useShortcut func() bool,
	shortcutMethod func(ctx context.Context, addr string) (shortcut.Method, net.IP),
	useDetour func() bool,
	allowHTTPSEverywhere func() bool,
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	allowPrivateHosts func() bool,
	lang func() string,
	reverseDNS func(addr string) (string, error),
	eventWithLabel func(category, action, label string),
	onDialError func(error, bool),
	onSucceedingProxy func(),
) (*Client, error) {
	// A small LRU to detect redirect loop
	rewriteLRU, err := lru.New(100)
	if err != nil {
		return nil, errors.New("Unable to create rewrite LRU: %v", err)
	}

	dialerSelector := chained.NewSelector([]chained.Dialer{})
	if err != nil {
		return nil, errors.New("Unable to create chained: %v", err)
	}

	client := &Client{
		configDir:                              configDir,
		requestTimeout:                         requestTimeout,
		dialer:                                 dialerSelector,
		disconnected:                           disconnected,
		proxyAll:                               proxyAll,
		useShortcut:                            useShortcut,
		shortcutMethod:                         shortcutMethod,
		useDetour:                              useDetour,
		onDialError:                            onDialError,
		onSucceedingProxy:                      onSucceedingProxy,
		allowHTTPSEverywhere:                   allowHTTPSEverywhere,
		user:                                   userConfig,
		rewriteToHTTPS:                         httpseverywhere.Default(),
		rewriteLRU:                             rewriteLRU,
		statsTracker:                           statsTracker,
		opsMap:                                 newOpsMap(),
		allowPrivateHosts:                      allowPrivateHosts,
		lang:                                   lang,
		reverseDNS:                             reverseDNS,
		eventWithLabel:                         eventWithLabel,
		httpListener:                           eventual.NewValue(),
		socksListener:                          eventual.NewValue(),
		DNSResolutionMapForDirectDialsEventual: eventual.NewValue(),
	}

	keepAliveIdleTimeout := chained.IdleTimeout - 5*time.Second

	client.proxy = proxy.New(&proxy.Opts{
		IdleTimeout: keepAliveIdleTimeout,
		Filter:      filters.FilterFunc(client.filter),
		OnError:     errorResponse,
		Dial:        client.dial,
	})
	client.iptool, _ = iptool.New()

	go client.cacheClientHellos()
	return client, nil
}

// Addr returns the address at which the client is listening with HTTP, blocking
// until the given timeout for an address to become available.
func Addr(timeout time.Duration) (interface{}, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := addr.Get(ctx)
	return result, err == nil
}

// Addr returns the address at which the client is listening with HTTP, blocking
// until the given timeout for an address to become available.
func (client *Client) Addr(timeout time.Duration) (interface{}, bool) {
	return Addr(timeout)
}

// Socks5Addr returns the address at which the client is listening with SOCKS5,
// blocking until the given timeout for an address to become available.
func Socks5Addr(timeout time.Duration) (interface{}, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := socksAddr.Get(ctx)
	return result, err == nil
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
	client.httpWg.Add(1)
	defer client.httpWg.Done()

	var err error
	var l net.Listener
	log.Debugf("About to listen at '%s'", requestedAddr)
	if l, err = net.Listen("tcp", requestedAddr); err != nil {
		log.Debugf("Error listening at '%s', fallback to default: %v", requestedAddr, err)
		requestedAddr = "127.0.0.1:0"
		log.Debugf("About to listen at '%s'", requestedAddr)
		if l, err = net.Listen("tcp", requestedAddr); err != nil {
			return fmt.Errorf("unable to listen: %q", err)
		}
	}
	defer l.Close()

	client.httpListener.Set(l)
	defer func() {
		client.httpListener.Reset()
	}()

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
			return fmt.Errorf("unable to accept connection: %v", err)
		}
		go client.handle(conn)
	}
}

// ListenAndServeSOCKS5 starts the SOCKS server listening at the specified
// address.
func (client *Client) ListenAndServeSOCKS5(requestedAddr string) error {
	client.socksWg.Add(1)
	defer client.socksWg.Done()

	var err error
	var l net.Listener
	if l, err = net.Listen("tcp", requestedAddr); err != nil {
		return fmt.Errorf("unable to listen: %q", err)
	}
	l = &optimisticListener{Listener: l}
	defer l.Close()

	client.socksListener.Set(l)
	defer func() {
		client.socksListener.Reset()
	}()

	listenAddr := l.Addr().String()
	socksAddr.Set(listenAddr)

	conf := &socks5.Config{
		HandleConnect: func(ctx context.Context, conn net.Conn, req *socks5.Request, replySuccess func(boundAddr net.Addr) error, replyError func(err error) error) error {
			op := ops.Begin("proxy")
			defer op.End()

			host := fmt.Sprintf("%v:%v", req.DestAddr.IP, req.DestAddr.Port)
			addr, err := client.reverseDNS(host)
			if err != nil {
				return op.FailIf(log.Errorf("Error performing reverseDNS for %v: %v", host, err))
			}
			errOnReply := replySuccess(nil)
			if errOnReply != nil {
				return op.FailIf(log.Errorf("Unable to reply success to SOCKS5 client: %v", errOnReply))
			}
			return op.FailIf(client.Connect(ctx, req.BufConn, conn, addr))
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("unable to create SOCKS5 server: %v", err)
	}

	log.Debugf("About to start SOCKS5 client proxy at %v", listenAddr)
	return server.Serve(l)
}

// Connects a downstream connection to the given origin and copies data bidirectionally.
// downstreamReader is a Reader that wraps downstream. Ideally, this is a buffered reader like
// from bufio.
func (client *Client) Connect(dialCtx context.Context, downstreamReader io.Reader, downstream net.Conn, origin string) error {
	return client.proxy.Connect(dialCtx, downstreamReader, downstream, origin)
}

// Configure updates the client's configuration. Configure can be called
// before or after ListenAndServe, and can be called multiple times. If
// no error occurred, then the new dialers are returned.
func (client *Client) Configure(proxies map[string]*commonconfig.ProxyConfig) []chained.Dialer {
	log.Debug("Configure() called")
	dialers, dialer, err := client.initDialers(proxies)
	if err != nil {
		log.Error(err)
		return nil
	}
	client.dialer = dialer
	chained.PersistSessionStates(client.configDir)
	chained.TrackStatsFor(dialers, client.configDir)
	return dialers
}

// Stop is called when the client is no longer needed. It closes the
// client listener and underlying dialer connection pool
func (client *Client) Stop() error {
	httpListener, _ := client.httpListener.Get(eventual.DontWait)
	socksListener, _ := client.socksListener.Get(eventual.DontWait)

	var httpError error
	var socksError error

	if httpListener != nil {
		httpError = httpListener.(net.Listener).Close()
		client.httpWg.Wait()
		addr.Reset()
	}
	if socksListener != nil {
		socksError = socksListener.(net.Listener).Close()
		client.socksWg.Wait()
		socksAddr.Reset()
	}

	if httpError != nil {
		return httpError
	}
	return socksError
}

var TimeoutWaitingForDNSResolutionMap = 5 * time.Second

func (client *Client) dial(ctx context.Context, isConnect bool, network, addr string) (conn net.Conn, err error) {
	op := ops.Begin("proxied_dialer")
	op.Set("local_proxy_type", "http")
	op.OriginPort(addr, "")
	defer op.End()

	// Fetch DNS resolution map, if any
	// XXX <01-04-2022, soltzen> Do this fetch now, so it won't be affected by
	// the context timeout of client.doDial()
	var dnsResolutionMapForDirectDials map[string]string

	ctx1, cancel1 := context.WithTimeout(ctx, TimeoutWaitingForDNSResolutionMap)
	defer cancel1()
	tmp, err := client.DNSResolutionMapForDirectDialsEventual.Get(ctx1)
	if err != nil {
		log.Debugf("Timed out before waiting for dnsResolutionMapEventual to be set")
		dnsResolutionMapForDirectDials = nil
	} else {
		dnsResolutionMapForDirectDials = tmp.(map[string]string)
	}

	ctx2, cancel2 := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel2()
	return client.doDial(op, ctx2, isConnect, addr, dnsResolutionMapForDirectDials)
}

// doDial is the ultimate place to dial an origin site. It takes following steps:
// * If the addr is in the proxied sites list or previously detected by detour as blocked, dial the site through proxies.
// * If proxyAll is on, dial the site through proxies.
// * If the host or port is configured not proxyable, dial directly.
// * If the site is allowed by shortcut, dial directly. If it failed before the deadline, try proxying.
// * Try dial the site directly with 1/5th of the requestTimeout, then try proxying.
func (client *Client) doDial(
	op *ops.Op,
	ctx context.Context,
	isCONNECT bool,
	addr string,
	dnsResolutionMapForDirectDials map[string]string) (net.Conn, error) {

	dialDirect := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if v, ok := dnsResolutionMapForDirectDials[addr]; ok {
			log.Debugf("Bypassed DNS resolution: dialing %v as %v", addr, v)
			conn, err := netx.DialContext(ctx, network, v)
			op.FailIf(err)
			return conn, err
		} else {
			conn, err := netx.DialContext(ctx, network, addr)
			op.FailIf(err)
			return conn, err
		}
	}

	dialProxied := func(ctx context.Context, _unused, addr string) (net.Conn, error) {
		op.Set("remotely_proxied", true)
		proto := chained.NetworkPersistent
		if isCONNECT {
			// UGLY HACK ALERT! In this case, we know we need to send a CONNECT request
			// to the chained server. We need to send that request from chained/dialer.go
			// though because only it knows about the authentication token to use.
			// We signal it to send the CONNECT here using the network transport argument
			// that is effectively always "tcp" in the end, but we look for this
			// special "transport" in the dialer and send a CONNECT request in that
			// case.
			proto = chained.NetworkConnect
		}
		start := time.Now()
		conn, err := client.dialer.DialContext(ctx, proto, addr)
		if log.IsTraceEnabled() {
			log.Tracef("Dialing proxy takes %v for %s", time.Since(start), addr)
		}
		if conn != nil {
			conn = &proxiedConn{conn}
		}
		return conn, err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.ToLower(strings.TrimSpace(host))
	routingRuleForDomain := domainrouting.RuleFor(host)

	if routingRuleForDomain == domainrouting.MustDirect {
		log.Debugf("Forcing direct to %v per domain routing rules (MustDirect)", host)
		op.Set("force_direct", true)
		op.Set("force_direct_reason", "routingrule")
		return dialDirect(ctx, "tcp", addr)
	}

	if shouldForceProxying() {
		log.Tracef("Proxying to %v because everything is forced to be proxied", addr)
		op.Set("force_proxied", true)
		op.Set("force_proxied_reason", "forceproxying")
		return dialProxied(ctx, "whatever", addr)
	}

	if routingRuleForDomain == domainrouting.MustProxy {
		log.Tracef("Proxying to %v per domain routing rules (MustProxy)", addr)
		op.Set("force_proxied", true)
		op.Set("force_proxied_reason", "routingrule")
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

	dialDirectForShortcut := func(ctx context.Context, network, addr string, ip net.IP) (net.Conn, error) {
		log.Debugf("Use shortcut (dial directly) for %v(%v)", addr, ip)
		op.Set("shortcut_direct", true)
		op.Set("shortcut_direct_ip", ip)
		op.Set("shortcut_origin", addr)
		return dialDirect(ctx, "tcp", addr)
	}

	switch domainrouting.RuleFor(host) {
	case domainrouting.Direct:
		log.Tracef("Directly dialing %v per domain routing rules (Direct)", addr)
		op.Set("force_direct", true)
		op.Set("force_direct_reason", "routingrule")
		return dialDirect(ctx, "tcp", addr)
	case domainrouting.Proxy:
		log.Tracef("Proxying to %v per domain routing rules (Proxy)", addr)
		op.Set("force_proxied", true)
		op.Set("force_proxied_reason", "routingrule")
		return dialProxied(ctx, "whatever", addr)
	}

	dl, _ := ctx.Deadline()
	// It's roughly requestTimeout (20s) / 5 = 4s to leave enough time
	// to try dialing via proxies. Not hardcode to 4s to avoid break test
	// code which may have a shorter requestTimeout.
	directTimeout := time.Until(dl) / 5
	cappedCTX, cancel := context.WithTimeout(ctx, directTimeout)
	defer cancel()

	dialDirectForDetour := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if client.useShortcut() {
			method, ip := client.shortcutMethod(cappedCTX, addr)
			switch method {
			case shortcut.Direct:
				// Arbitrarily have a larger timeout if the address is eligible for shortcut.
				shortcutCTX, cancel := context.WithTimeout(ctx, directTimeout*2)
				defer cancel()
				return dialDirectForShortcut(shortcutCTX, network, addr, ip)
			case shortcut.Proxy:
				return dialProxied(ctx, "whatever", addr)
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
			method, ip := client.shortcutMethod(ctx, addr)
			switch method {
			case shortcut.Direct:
				// Don't cap the context if the address is eligible for shortcut.
				conn, err = dialDirectForShortcut(ctx, network, addr, ip)
				if err == nil {
					return conn, err
				}
			case shortcut.Proxy:
				return dialProxied(ctx, network, addr)
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
	return fmt.Errorf("port %d not proxyable", port)
}

// isAddressProxyable largely replicates the logic in the old PAC file
func (client *Client) isAddressProxyable(addr string) error {
	if client.allowPrivateHosts() {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("unable to split host and port for %v, considering private: %v", addr, err)
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
			if strings.HasSuffix(host, ".onion") {
				return fmt.Errorf("%v ends in .onion, considering private", host)
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
		return 0, fmt.Errorf("unable to determine port for address %v: %v", addr, err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return 0, fmt.Errorf("unable to parse port %v for address %v: %v", addr, port, err)
	}
	return port, nil
}

func errorResponse(_ *filters.ConnectionState, req *http.Request, _ bool, err error) *http.Response {
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
		Body:       io.NopCloser(bytes.NewBuffer(htmlerr)),
		StatusCode: http.StatusServiceUnavailable,
	}
}

// initDialers takes hosts from cfg.ChainedServers and it uses them to create a
// new dialer. Returns the new dialers.
func (client *Client) initDialers(proxies map[string]*commonconfig.ProxyConfig) ([]chained.Dialer, *chained.Selector, error) {
	if len(proxies) == 0 {
		return nil, nil, fmt.Errorf("no chained servers configured, not initializing dialers")
	}
	configDir := client.configDir
	chained.PersistSessionStates(configDir)
	dialers := chained.CreateDialers(configDir, proxies, client.user)
	dialer := chained.NewSelector(dialers)
	return dialers, dialer, nil
}

// Creates a local server to capture client hello messages from the browser and
// caches them.
func (client *Client) cacheClientHellos() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Try to snag a hello from the browser.
	chained.ActivelyObtainBrowserHello(ctx, client.configDir)
}
