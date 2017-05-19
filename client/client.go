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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/detour"
	"github.com/getlantern/easylist"
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
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/shortcut"
	"github.com/getlantern/flashlight/status"
)

var (
	log = golog.LoggerFor("flashlight.client")

	ServiceType service.Type = "flashlight.client"

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

type ConfigOpts struct {
	UseShortcut       bool
	UseDetour         bool
	AllowPrivateHosts bool
	ProToken          string
	DeviceID          string
	HTTPProxyAddr     string
	Socks5ProxyAddr   string
	Proxies           map[string]*chained.ChainedServerInfo

	StatsTracker common.StatsTracker
}

func (o *ConfigOpts) For() service.Type {
	return ServiceType
}

func (o *ConfigOpts) Complete() string {
	if o.StatsTracker == nil {
		return "missing StatsTracker"
	}
	if o.HTTPProxyAddr == "" {
		return "missing HTTPProxyAddr"
	}
	if o.Socks5ProxyAddr == "" {
		return "missing Socks5ProxyAddr"
	}
	if o.DeviceID == "" {
		return "missing DeviceID"
	}
	if len(o.Proxies) == 0 {
		return "missing Proxies"
	}
	return ""
}

type ProxyType string

var (
	HTTPProxy   ProxyType = "http-proxy"
	Socks5Proxy ProxyType = "socks5-proxy"
)

type Message struct {
	ProxyType ProxyType
	Addr      string
}

func (m Message) ValidMessageFrom(t service.Type) bool {
	return t == ServiceType && m.Addr != ""
}

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

	httpListener   net.Listener
	socks5Listener net.Listener

	useShortcut       bool
	useDetour         bool
	proToken          string
	allowPrivateHosts bool

	httpProxyAddr   string
	socks5ProxyAddr string

	easylist       easylist.List
	rewriteToHTTPS httpseverywhere.Rewrite
	statsTracker   common.StatsTracker
	publisher      service.Publisher
	isPrivateAddr  func(*net.IPAddr) bool

	chStop chan bool
}

func New() service.Impl {
	keepAliveIdleTimeout := chained.IdleTimeout - 5*time.Second
	c := &Client{
		bal:            balancer.New(),
		rewriteToHTTPS: httpseverywhere.Default(),
		chStop:         make(chan bool),
	}

	c.interceptCONNECT = proxy.CONNECT(keepAliveIdleTimeout, buffers.Pool, false, c.dialCONNECT)
	c.interceptHTTP = proxy.HTTP(false, keepAliveIdleTimeout, nil, nil, errorResponse, c.dialHTTP)
	iptool, err := iptool.New()
	if err != nil {
		log.Errorf("Error creating iptool, assuming all addresses non-private: %v", err)
		c.isPrivateAddr = func(*net.IPAddr) bool { return false }
	} else {
		c.isPrivateAddr = iptool.IsPrivate
	}
	return c

}

func (c *Client) GetType() service.Type {
	return ServiceType
}

func (c *Client) Reconfigure(p service.Publisher, opts service.ConfigOpts) {
	c.publisher = p
	o := opts.(*ConfigOpts)
	c.useShortcut = o.UseShortcut
	c.useDetour = o.UseDetour
	c.proToken = o.ProToken
	c.statsTracker = o.StatsTracker
	c.allowPrivateHosts = o.AllowPrivateHosts
	c.httpProxyAddr = o.HTTPProxyAddr
	c.socks5ProxyAddr = o.Socks5ProxyAddr
	c.initEasyList()
	err := c.initBalancer(o.Proxies, o.DeviceID)
	if err != nil {
		log.Error(err)
	}
}

func (c *Client) Start() {
	c.reportProxyLocationLoop()
	go func() {
		err := c.listenAndServeHTTP()
		if err != nil {
			log.Error(err)
		}
	}()
	go func() {
		err := c.listenAndServeSOCKS5()
		if err != nil {
			log.Error(err)
		}
	}()
}

// Stop is called when the client is no longer needed. It closes the
// client listener and underlying dialer connection pool
func (c *Client) Stop() {
	close(c.chStop)
	if c.httpListener != nil {
		c.httpListener.Close()
	}
	if c.socks5Listener != nil {
		c.socks5Listener.Close()
	}
}

type allowAllEasyList struct{}

func (l allowAllEasyList) Allow(*http.Request) bool {
	return true
}

func (c *Client) initEasyList() {
	defer func() {
		if c.easylist == nil {
			log.Debugf("Not using easylist")
			c.easylist = allowAllEasyList{}
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
	c.easylist = list
	log.Debug("Initialized easylist")
}

func (c *Client) reportProxyLocationLoop() {
	ch := c.bal.OnActiveDialer()
	var activeProxy string
	go func() {
		for {
			select {
			case proxy := <-ch:
				if proxy.Name() == activeProxy {
					continue
				}
				activeProxy = proxy.Name()
				loc := proxyLoc(activeProxy)
				if loc == nil {
					log.Errorf("Couldn't find location for %s", activeProxy)
					continue
				}
				c.statsTracker.SetActiveProxyLocation(
					loc.city,
					loc.country,
					loc.countryCode,
				)
			case <-c.chStop:
				return
			}
		}
	}()
}

// listenAndServeHTTP makes the client listen for HTTP connections at a the given
// address or, if a blank address is given, at a random port on localhost.
// onListeningFn is a callback that gets invoked as soon as the server is
// accepting TCP connections.
func (c *Client) listenAndServeHTTP() error {
	var err error
	var l net.Listener
	if l, err = net.Listen("tcp", c.httpProxyAddr); err != nil {
		return fmt.Errorf("Unable to listen: %q", err)
	}

	c.httpListener = l
	listenAddr := l.Addr().String()
	c.publisher.Publish(Message{HTTPProxy, listenAddr})
	httpServer := &http.Server{
		ReadTimeout:  c.readTimeout,
		WriteTimeout: c.writeTimeout,
		Handler:      c,
		ErrorLog:     log.AsStdLogger(),
	}

	log.Debugf("About to start HTTP client proxy at %v", listenAddr)
	return httpServer.Serve(l)
}

// listenAndServeSOCKS5 starts the SOCKS server listening at the specified
// address.
func (c *Client) listenAndServeSOCKS5() error {
	var err error
	var l net.Listener
	if l, err = net.Listen("tcp", c.socks5ProxyAddr); err != nil {
		return fmt.Errorf("Unable to listen: %q", err)
	}
	c.socks5Listener = l
	listenAddr := l.Addr().String()
	c.publisher.Publish(Message{Socks5Proxy, listenAddr})

	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			port, portErr := c.portForAddress(addr)
			if portErr != nil {
				return nil, portErr
			}
			return c.doDial(ctx, true, addr, port)
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("Unable to create SOCKS5 server: %v", err)
	}

	log.Debugf("About to start SOCKS5 client proxy at %v", listenAddr)
	return server.Serve(l)
}

func (c *Client) proxiedDialer(orig func(network, addr string) (net.Conn, error)) func(network, addr string) (net.Conn, error) {
	detourDialer := detour.Dialer(orig)

	return func(network, addr string) (net.Conn, error) {
		op := ops.Begin("proxied_dialer")
		defer op.End()

		var proxied func(network, addr string) (net.Conn, error)
		if c.useDetour {
			op.Set("detour", true)
			proxied = detourDialer
		} else {
			op.Set("detour", false)
			proxied = orig
		}

		start := time.Now()
		conn, err := proxied(network, addr)
		if log.IsTraceEnabled() {
			log.Tracef("Dialing proxy takes %v for %s", time.Since(start), addr)
		}
		return conn, op.FailIf(err)
	}
}

func (c *Client) dialCONNECT(network, addr string) (conn net.Conn, err error) {
	return c.dial(true, network, addr)
}

func (c *Client) dialHTTP(network, addr string) (conn net.Conn, err error) {
	return c.dial(false, network, addr)
}

func (c *Client) dial(isConnect bool, network, addr string) (conn net.Conn, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), getRequestTimeout())
	defer cancel()
	port, err := c.portForAddress(addr)
	if err != nil {
		return nil, err
	}
	return c.doDial(ctx, isConnect, addr, port)
}

func (c *Client) doDial(ctx context.Context, isCONNECT bool, addr string, port int) (net.Conn, error) {
	// Establish outbound connection
	if err := c.shouldSendToProxy(addr, port); err != nil {
		log.Debugf("%v, sending directly to %v", err, addr)
		// Use netx because on Android, we need a special protected dialer
		return netx.DialContext(ctx, "tcp", addr)
	}
	if c.useShortcut && shortcut.Allow(addr) {
		log.Debugf("Use shortcut (dial directly) for %v", addr)
		return netx.DialContext(ctx, "tcp", addr)
	}

	d := c.proxiedDialer(func(network, addr string) (net.Conn, error) {
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
		return c.bal.Dial(proto, addr)
	})
	// TODO: pass context down to all layers.
	chDone := make(chan bool)
	var conn net.Conn
	var err error
	go func() {
		conn, err = d("tcp", addr)
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

func (c *Client) shouldSendToProxy(addr string, port int) error {
	err := c.isPortProxyable(port)
	if err == nil {
		err = c.isAddressProxyable(addr)
	}
	return err
}

func (c *Client) isPortProxyable(port int) error {
	for _, proxiedPort := range proxiedCONNECTPorts {
		if port == proxiedPort {
			return nil
		}
	}
	return fmt.Errorf("Port %d not proxyable", port)
}

// isAddressProxyable largely replicates the logic in the old PAC file
func (c *Client) isAddressProxyable(addr string) error {
	if c.allowPrivateHosts {
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
	if c.isPrivateAddr(ipAddrToCheck) {
		return fmt.Errorf("IP %v is private", host)
	}
	return nil
}

func (c *Client) portForAddress(addr string) (int, error) {
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
