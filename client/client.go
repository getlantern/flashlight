package client

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/detour"
	"github.com/getlantern/easylist"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/go-socks5"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/httpseverywhere"
	"github.com/getlantern/iptool"
	"github.com/getlantern/netx"
	"github.com/getlantern/proxy"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/status"
)

var (
	log = golog.LoggerFor("flashlight.client")

	addr                = eventual.NewValue()
	socksAddr           = eventual.NewValue()
	proxiedCONNECTPorts = []int{
		// Standard HTTP(S) ports
		80, 443,
		// Common unprivileged HTTP(S) ports
		8080, 8443,
		// XMPP
		5222, 5223, 5224,
		// Android
		5228, 5229,
		// udpgw
		7300,
		// Google Hangouts TCP Ports (see https://support.google.com/a/answer/1279090?hl=en)
		19305, 19306, 19307, 19308, 19309,
	}

	// Set a hard limit when processing proxy requests. Should be short enough to
	// avoid applications bypassing Lantern.
	// Chrome has a 30s timeout before marking proxy as bad.
	requestTimeout = int64(20 * time.Second)

	// See http://stackoverflow.com/questions/106179/regular-expression-to-match-dns-hostname-or-ip-address
	validHostnameRegex = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)
)

// Client is an HTTP proxy that accepts connections from local programs and
// proxies these via remote flashlight servers.
type Client struct {
	// readTimeout: (optional) timeout for read ops
	readTimeout time.Duration

	// writeTimeout: (optional) timeout for write ops
	writeTimeout time.Duration

	// Balanced CONNECT dialers.
	bal *balancer.Balancer

	interceptCONNECT proxy.Interceptor
	interceptHTTP    proxy.Interceptor

	l net.Listener

	allowShortcut  func(addr string) (bool, net.IP)
	useDetour      func() bool
	proTokenGetter func() string

	easylist       easylist.List
	rewriteToHTTPS httpseverywhere.Rewrite

	statsTracker stats.StatsTracker

	iptool            iptool.Tool
	allowPrivateHosts func() bool
	lang              func() string
	adSwapTargetURL   func() string
}

// NewClient creates a new client that does things like starts the HTTP and
// SOCKS proxies. It take a function for determing whether or not to proxy
// all traffic, and another function to get Lantern Pro token when required.
func NewClient(
	allowShortcut func(addr string) (bool, net.IP),
	useDetour func() bool,
	proTokenGetter func() string,
	statsTracker stats.StatsTracker,
	allowPrivateHosts func() bool,
	lang func() string,
	adSwapTargetURL func() string,
) (*Client, error) {
	client := &Client{
		bal:               balancer.New(),
		allowShortcut:     allowShortcut,
		useDetour:         useDetour,
		proTokenGetter:    proTokenGetter,
		rewriteToHTTPS:    httpseverywhere.Default(),
		statsTracker:      statsTracker,
		allowPrivateHosts: allowPrivateHosts,
		lang:              lang,
		adSwapTargetURL:   adSwapTargetURL,
	}

	keepAliveIdleTimeout := chained.IdleTimeout - 5*time.Second
	client.interceptCONNECT = proxy.CONNECT(keepAliveIdleTimeout, buffers.Pool, false, client.dialCONNECT)
	client.interceptHTTP = proxy.HTTP(false, keepAliveIdleTimeout, nil, nil, errorResponse, client.dialHTTP)
	// TODO: turn it to a config option
	if runtime.GOOS == "android" {
		client.easylist = allowAllEasyList{}
	} else {
		client.initEasyList()
	}
	client.reportProxyLocationLoop()
	var err error
	client.iptool, err = iptool.New()
	if err != nil {
		return nil, errors.New("Unable to initialize iptool: %v", err)
	}
	return client, nil
}

type allowAllEasyList struct{}

func (l allowAllEasyList) Allow(*http.Request) bool {
	return true
}

func (client *Client) initEasyList() {
	defer func() {
		if client.easylist == nil {
			log.Debugf("Not using easylist")
			client.easylist = allowAllEasyList{}
		}
	}()
	log.Debug("Initializing easylist")
	path, err := InConfigDir("", "easylist.txt")
	if err != nil {
		log.Errorf("Unable to get config path: %v", err)
		return
	}
	list, err := easylist.Open(path, 1*time.Hour)
	if err != nil {
		log.Errorf("Unable to open easylist: %v", err)
		return
	}
	client.easylist = list
	log.Debug("Initialized easylist")
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
			activeProxy = proxy.Name()
			loc := proxyLoc(activeProxy)
			if loc == nil {
				log.Errorf("Couldn't find location for %s", activeProxy)
				continue
			}
			client.statsTracker.SetActiveProxyLocation(
				loc.city,
				loc.country,
				loc.countryCode,
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
	log.Debug("About to listen")
	if requestedAddr == "" {
		requestedAddr = "127.0.0.1:0"
	}

	var err error
	var l net.Listener
	if l, err = net.Listen("tcp", requestedAddr); err != nil {
		return fmt.Errorf("Unable to listen: %q", err)
	}

	client.l = l
	listenAddr := l.Addr().String()
	addr.Set(listenAddr)
	onListeningFn()

	httpServer := &http.Server{
		ReadTimeout:  client.readTimeout,
		WriteTimeout: client.writeTimeout,
		Handler:      client,
		ErrorLog:     log.AsStdLogger(),
	}

	log.Debugf("About to start HTTP client proxy at %v", listenAddr)
	for {
		start := time.Now()
		err := httpServer.Serve(l)
		if time.Since(start) < 10*time.Second {
			return err
		}
		log.Debugf("Error serving HTTP client proxy at %v, restarting: %v", listenAddr, err)
		time.Sleep(100 * time.Millisecond)
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
	listenAddr := l.Addr().String()
	socksAddr.Set(listenAddr)

	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			op := ops.Begin("proxied_dialer")
			op.Set("local_proxy_type", "socks5")
			defer op.End()
			return client.doDial(op, ctx, true, addr)
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
// before or after ListenAndServe, and can be called multiple times.
func (client *Client) Configure(proxies map[string]*chained.ChainedServerInfo, deviceID string) {
	log.Debug("Configure() called")
	err := client.initBalancer(proxies, deviceID)
	if err != nil {
		log.Error(err)
	}
}

// Stop is called when the client is no longer needed. It closes the
// client listener and underlying dialer connection pool
func (client *Client) Stop() error {
	return client.l.Close()
}

func (client *Client) dialCONNECT(network, addr string) (conn net.Conn, err error) {
	return client.dial(true, network, addr)
}

func (client *Client) dialHTTP(network, addr string) (conn net.Conn, err error) {
	return client.dial(false, network, addr)
}

func (client *Client) dial(isConnect bool, network, addr string) (conn net.Conn, err error) {
	op := ops.Begin("proxied_dialer")
	op.Set("local_proxy_type", "http")
	defer op.End()
	return client.doDial(op, context.Background(), isConnect, addr)
}

// doDial is the ultimate place to dial an origin site. It takes following steps:
// * If the addr is in the proxied sites list or previously detected by detour as blocked, dial the site through proxies.
// * If proxyAll is on, dial the site through proxies.
// * If the host or port is configured not proxyable, dial directly.
// * If the site is allowed by shortcut, dial directly. If it failed before the deadline, try proxying.
// * Try dial the site directly with 1/5th of the requestTimeout, then try proxying.
func (client *Client) doDial(op *ops.Op, ctx context.Context, isCONNECT bool, addr string) (net.Conn, error) {
	port, err := client.portForAddress(addr)
	if err != nil {
		return nil, err
	}

	newCTX, cancel := context.WithTimeout(ctx, getRequestTimeout())
	defer cancel()
	op.Origin(addr, "")

	if err := client.shouldSendToProxy(addr, port); err != nil {
		log.Debugf("%v, sending directly to %v", err, addr)
		op.Set("force_direct", true)
		op.Set("force_direct_reason", err.Error())
		// Use netx because on Android, we need a special protected dialer, same below
		return netx.DialContext(newCTX, "tcp", addr)
	}

	dialer := client.getDialer(op, isCONNECT)
	c, e := dialer(newCTX, "tcp", addr)
	return c, op.FailIf(e)
}

func (client *Client) getDialer(op *ops.Op, isCONNECT bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	directDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if allow, ip := client.allowShortcut(addr); allow {
			log.Debugf("Use shortcut (dial directly) for %v(%v)", addr, ip)
			op.Set("shortcut_direct", true)
			op.Set("shortcut_direct_ip", ip)
			return netx.DialContext(ctx, "tcp", addr)
		}
		dl, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("context has no deadline")
		}
		// It's roughly requestTimeout (20s) / 5 = 4s to leave enough time
		// to try detour. Not hardcode to 4s to avoid break test code which may
		// have a shorter requestTimeout.
		timeout := dl.Sub(time.Now()) / 5
		newCTX, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return netx.DialContext(newCTX, "tcp", addr)
	}

	proxiedDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		proto := "persistent"
		if isCONNECT {
			// UGLY HACK ALERT! In this case, we know we need to send a CONNECT request
			// to the chained server. We need to send that request from chained/dialer.go
			// though because only it knows about the authentication token to use.
			// We signal it to send the CONNECT here using the network transport argument
			// that is effectively always "tcp" in the end, but we look for this
			// special "transport" in the dialer and send a CONNECT request in that
			// case.
			proto = "connect"
		}
		// TODO: pass context down to all layers.
		chDone := make(chan bool)
		var conn net.Conn
		var err error
		go func() {
			start := time.Now()
			conn, err = client.bal.Dial(proto, addr)
			if log.IsTraceEnabled() {
				log.Tracef("Dialing proxy takes %v for %s", time.Since(start), addr)
			}
			chDone <- true
		}()
		select {
		case <-chDone:
			return conn, err
		case <-ctx.Done():
			go func() {
				<-chDone
				if conn != nil {
					log.Debugf("Connection to %s established too late, closing", addr)
					conn.Close()
				}
			}()
			return nil, ctx.Err()
		}
	}

	var dialer func(ctx context.Context, network, addr string) (net.Conn, error)
	if client.useDetour() {
		op.Set("detour", true)
		dialer = detour.Dialer(directDialer, proxiedDialer)
	} else {
		op.Set("detour", false)
		dialer = proxiedDialer
	}
	return dialer
}

func (client *Client) shouldSendToProxy(addr string, port int) error {
	err := client.isPortProxyable(port)
	if err == nil {
		err = client.isAddressProxyable(addr)
	}
	return err
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

// InConfigDir returns the path of the specified file name in the Lantern
// configuration directory, using an alternate base configuration directory
// if necessary for things like testing.
func InConfigDir(configDir string, filename string) (string, error) {
	cdir := configDir

	if cdir == "" {
		cdir = appdir.General("Lantern")
	}

	log.Debugf("Using config dir %v", cdir)
	if _, err := os.Stat(cdir); err != nil {
		if os.IsNotExist(err) {
			// Create config dir
			if err := os.MkdirAll(cdir, 0750); err != nil {
				return "", fmt.Errorf("Unable to create configdir at %s: %s", cdir, err)
			}
		}
	}

	return filepath.Join(cdir, filename), nil
}

func errorResponse(req *http.Request, err error) *http.Response {
	var htmlerr []byte

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

	res := &http.Response{
		Body: ioutil.NopCloser(bytes.NewBuffer(htmlerr)),
	}
	res.StatusCode = http.StatusServiceUnavailable
	return res
}

func getRequestTimeout() time.Duration {
	return time.Duration(atomic.LoadInt64(&requestTimeout))
}
