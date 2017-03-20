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
	"strconv"
	"sync/atomic"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/detour"
	"github.com/getlantern/easylist"
	"github.com/getlantern/eventual"
	"github.com/getlantern/go-socks5"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/netx"
	"github.com/getlantern/proxy"

	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/shortcut"
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
)

// Client is an HTTP proxy that accepts connections from local programs and
// proxies these via remote flashlight servers.
type Client struct {
	// readTimeout: (optional) timeout for read ops
	readTimeout time.Duration

	// writeTimeout: (optional) timeout for write ops
	writeTimeout time.Duration

	// Balanced CONNECT dialers.
	bal eventual.Value

	interceptCONNECT proxy.Interceptor
	interceptHTTP    proxy.Interceptor

	l net.Listener

	proxyAll       func() bool
	proTokenGetter func() string

	easylist easylist.List
}

// NewClient creates a new client that does things like starts the HTTP and
// SOCKS proxies. It take a function for determing whether or not to proxy
// all traffic, and another function to get Lantern Pro token when required.
func NewClient(proxyAll func() bool, proTokenGetter func() string) *Client {
	client := &Client{
		bal:            eventual.NewValue(),
		proxyAll:       proxyAll,
		proTokenGetter: proTokenGetter,
	}

	keepAliveIdleTimeout := idleTimeout - 5*time.Second
	client.interceptCONNECT = proxy.CONNECT(keepAliveIdleTimeout, buffers.Pool, false, client.dialCONNECT)
	client.interceptHTTP = proxy.HTTP(false, keepAliveIdleTimeout, nil, nil, errorResponse, client.dialHTTP)
	client.initEasyList()
	return client
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

// ListenAndServeHTTP makes the client listen for HTTP connections at a the given
// address or, if a blank address is given, at a random port on localhost.
// onListeningFn is a callback that gets invoked as soon as the server is
// accepting TCP connections.
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
	return httpServer.Serve(l)
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
			port, portErr := client.portForAddress(addr)
			if portErr != nil {
				return nil, portErr
			}
			return client.doDial(ctx, true, addr, port)
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

func (client *Client) proxiedDialer(orig func(network, addr string) (net.Conn, error)) func(network, addr string) (net.Conn, error) {
	detourDialer := detour.Dialer(orig)

	return func(network, addr string) (net.Conn, error) {
		op := ops.Begin("proxied_dialer")
		defer op.End()

		var proxied func(network, addr string) (net.Conn, error)
		if client.proxyAll() {
			op.Set("detour", false)
			proxied = orig
		} else {
			op.Set("detour", true)
			proxied = detourDialer
		}

		start := time.Now()
		conn, err := proxied(network, addr)
		if log.IsTraceEnabled() {
			log.Tracef("Dialing proxy takes %v for %s", time.Since(start), addr)
		}
		return conn, op.FailIf(err)
	}
}

func (client *Client) dialCONNECT(network, addr string) (conn net.Conn, err error) {
	return client.dial(true, network, addr)
}

func (client *Client) dialHTTP(network, addr string) (conn net.Conn, err error) {
	return client.dial(false, network, addr)
}

func (client *Client) dial(isConnect bool, network, addr string) (conn net.Conn, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), getRequestTimeout())
	defer cancel()
	port, err := client.portForAddress(addr)
	if err != nil {
		return nil, err
	}
	return client.doDial(ctx, isConnect, addr, port)
}

func (client *Client) doDial(ctx context.Context, isCONNECT bool, addr string, port int) (net.Conn, error) {
	// Establish outbound connection
	if !client.shouldSendToProxy(addr, port) {
		log.Tracef("Port not allowed, bypassing proxy and sending request directly to %v", addr)
		// Use netx because on Android, we need a special protected dialer
		return netx.DialContext(ctx, "tcp", addr)
	}
	if !client.proxyAll() && shortcut.Allow(addr) {
		log.Tracef("Use shortcut (dial directly) for %v", addr)
		return netx.DialContext(ctx, "tcp", addr)
	}

	d := client.proxiedDialer(func(network, addr string) (net.Conn, error) {
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
		return bal.Dial(proto, addr)
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

func (client *Client) shouldSendToProxy(addr string, port int) bool {
	for _, proxiedPort := range proxiedCONNECTPorts {
		if port == proxiedPort {
			return true
		}
	}
	return false
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
