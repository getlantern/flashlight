package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/proxy"
	"github.com/getlantern/keyman"

	"gopkg.in/getlantern/tlsdialer.v1"
)

const (
	CONNECT = "CONNECT" // HTTP CONNECT method

	REVERSE_PROXY_FLUSH_INTERVAL = 250 * time.Millisecond

	X_FLASHLIGHT_QOS = "X-Flashlight-QOS"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Client is an HTTP proxy that accepts connections from local programs and
// proxies these via remote flashlight servers.
type Client struct {
	// Addr: listen address in form of host:port
	Addr string

	// ReadTimeout: (optional) timeout for read ops
	ReadTimeout time.Duration

	// WriteTimeout: (optional) timeout for write ops
	WriteTimeout time.Duration

	cfg                *ClientConfig
	cfgMutex           sync.RWMutex
	servers            []*server
	totalServerWeights int
}

// ListenAndServe makes the client listen for HTTP connections
func (client *Client) ListenAndServe() error {
	httpServer := &http.Server{
		Addr:         client.Addr,
		ReadTimeout:  client.ReadTimeout,
		WriteTimeout: client.WriteTimeout,
		Handler:      client,
	}

	log.Debugf("About to start client (http) proxy at %s", client.Addr)
	return httpServer.ListenAndServe()
}

// Configure updates the client's configuration.  Configure can be called
// before or after ListenAndServe, and can be called multiple times.  The
// optional enproxyConfigs parameter allows explicitly specifying enproxy
// configurations for the servers in ClientConfig in lieu of building them based
// on the ServerInfo in ClientConfig (mostly useful for testing).
func (client *Client) Configure(cfg *ClientConfig, enproxyConfigs []*enproxy.Config) {
	client.cfgMutex.Lock()
	defer client.cfgMutex.Unlock()

	log.Debug("Configure() called")
	if client.cfg != nil && reflect.DeepEqual(client.cfg, cfg) {
		log.Debugf("Client configuration unchanged")
		return
	}

	if client.cfg == nil {
		log.Debugf("Client configuration initialized")
	} else {
		log.Debugf("Client configuration changed")
	}

	client.cfg = cfg

	verifiedSets := make(map[string]*verifiedMasqueradeSet)
	testServer := cfg.highestQosServer()

	for key, masqueradeSet := range cfg.MasqueradeSets {
		verifiedSets[key] = newVerifiedMasqueradeSet(testServer, masqueradeSet)
	}

	// Configure servers
	client.servers = make([]*server, len(cfg.Servers))
	i := 0
	for _, serverInfo := range cfg.Servers {
		var enproxyConfig *enproxy.Config
		if enproxyConfigs != nil {
			enproxyConfig = enproxyConfigs[i]
		}
		client.servers[i] = serverInfo.buildServer(
			cfg.ShouldDumpHeaders,
			verifiedSets[serverInfo.MasqueradeSet],
			enproxyConfig)
		i = i + 1
	}

	// Calculate total server weights
	client.totalServerWeights = 0
	for _, server := range client.servers {
		client.totalServerWeights = client.totalServerWeights + server.info.Weight
	}
}

// highestQos finds the server with the highest reported quality of service.
func (cfg *ClientConfig) highestQosServer() *ServerInfo {
	highest := 0
	info := &ServerInfo{}
	for _, serverInfo := range cfg.Servers {
		if serverInfo.QOS > highest {
			highest = serverInfo.QOS
			info = serverInfo
		}
	}
	return info
}

// ServeHTTP implements the method from interface http.Handler
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	server := client.randomServer(req)
	log.Debugf("Using server %s to handle request for %s", server.info.Host, req.RequestURI)
	if req.Method == CONNECT {
		log.Debug("Handling CONNECT request")
		server.getEnproxyConfig().Intercept(resp, req)
	} else {
		log.Debug("Handling plain text HTTP request")
		server.reverseProxy.ServeHTTP(resp, req)
	}
}

// randomServer picks a random server from the list of servers, with higher
// weight servers more likely to be picked.  If the request includes our
// custom QOS header, only servers whose QOS meets or exceeds the requested
// value are considered for inclusion.  However, if no servers meet the QOS
// requirement, the last server in the list will be used by default.
func (client *Client) randomServer(req *http.Request) *server {
	targetQOS := client.targetQOS(req)

	servers, totalServerWeights := client.getServers()

	// Pick a random server using a target value between 0 and the total server weights
	t := rand.Intn(totalServerWeights)
	aw := 0
	for i, server := range servers {
		if i == len(servers)-1 {
			// Last server, use it irrespective of target QOS
			return server
		}
		aw = aw + server.info.Weight
		if server.info.QOS < targetQOS {
			// QOS too low, exclude server from rotation
			t = t + server.info.Weight
			continue
		}
		if aw > t {
			// We've reached our random target value, use this server
			return server
		}
	}

	// We should never reach this
	panic("No server found!")
}

// targetQOS determines the target quality of service given the X-Flashlight-QOS
// header if available, else returns 0.
func (client *Client) targetQOS(req *http.Request) int {
	requestedQOS := req.Header.Get(X_FLASHLIGHT_QOS)
	if requestedQOS != "" {
		rqos, err := strconv.Atoi(requestedQOS)
		if err == nil {
			return rqos
		}
	}

	return 0
}

func (client *Client) getServers() ([]*server, int) {
	client.cfgMutex.RLock()
	defer client.cfgMutex.RUnlock()
	return client.servers, client.totalServerWeights
}

// ServerInfo captures configuration information for an upstream server
type ServerInfo struct {
	// Host: the host (e.g. getiantem.org)
	Host string

	// Port: the port (e.g. 443)
	Port int

	// MasqueradeSet: the name of the masquerade set from ClientConfig that
	// contains masquerade hosts to use for this server.
	MasqueradeSet string

	// InsecureSkipVerify: if true, server's certificate is not verified.
	InsecureSkipVerify bool

	// DialTimeoutMillis: how long to wait on dialing server before timing out
	// (defaults to 5 seconds)
	DialTimeoutMillis int

	// KeepAliveMillis: interval for TCP keepalives (defaults to 70 seconds)
	KeepAliveMillis int

	// Weight: relative weight versus other servers (for round-robin)
	Weight int

	// QOS: relative quality of service offered.  Should be >= 0, with higher
	// values indicating higher QOS.
	QOS int
}

// buildServer builds a server configured from this serverInfo using the given
// enproxy.Config if provided.
func (serverInfo *ServerInfo) buildServer(shouldDumpHeaders bool, masquerades *verifiedMasqueradeSet, enproxyConfig *enproxy.Config) *server {
	weight := serverInfo.Weight
	if weight == 0 {
		weight = 100
	}

	server := &server{
		info:          serverInfo,
		masquerades:   masquerades,
		enproxyConfig: enproxyConfig,
	}

	server.reverseProxy = server.buildReverseProxy(shouldDumpHeaders)

	return server
}

func (serverInfo *ServerInfo) buildEnproxyConfig(masquerade *Masquerade) *enproxy.Config {
	dialTimeout := time.Duration(serverInfo.DialTimeoutMillis) * time.Millisecond
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	keepAlive := time.Duration(serverInfo.KeepAliveMillis) * time.Millisecond
	if keepAlive == 0 {
		keepAlive = 70 * time.Second
	}

	return &enproxy.Config{
		DialProxy: func(addr string) (net.Conn, error) {
			// Note - we need to suppress the sending of the ServerName in the
			// client handshake to make host-spoofing work with Fastly.  If the
			// client Hello includes a server name, Fastly checks to make sure
			// that this matches the Host header in the HTTP request and if they
			// don't match, it returns a 400 Bad Request error.
			sendServerNameExtension := false

			return tlsdialer.DialWithDialer(
				&net.Dialer{
					Timeout:   dialTimeout,
					KeepAlive: keepAlive,
				},
				"tcp",
				serverInfo.addressForServer(masquerade),
				sendServerNameExtension,
				serverInfo.tlsConfig(masquerade))
		},
		NewRequest: func(upstreamHost string, method string, body io.Reader) (req *http.Request, err error) {
			if upstreamHost == "" {
				// No specific host requested, use configured one
				upstreamHost = serverInfo.Host
			}
			return http.NewRequest(method, "http://"+upstreamHost+"/", body)
		},
	}
}

// Get the address to dial for reaching the server
func (serverInfo *ServerInfo) addressForServer(masquerade *Masquerade) string {
	return fmt.Sprintf("%s:%d", serverInfo.serverHost(masquerade), serverInfo.Port)
}

func (serverInfo *ServerInfo) serverHost(masquerade *Masquerade) string {
	serverHost := serverInfo.Host
	if masquerade != nil && masquerade.Domain != "" {
		serverHost = masquerade.Domain
	}
	return serverHost
}

// Build a tls.Config for dialing the upstream host
func (serverInfo *ServerInfo) tlsConfig(masquerade *Masquerade) *tls.Config {
	tlsConfig := &tls.Config{
		ClientSessionCache: tls.NewLRUClientSessionCache(1000),
		InsecureSkipVerify: serverInfo.InsecureSkipVerify,
	}

	if masquerade != nil && masquerade.RootCA != "" {
		caCert, err := keyman.LoadCertificateFromPEMBytes([]byte(masquerade.RootCA))
		if err != nil {
			log.Fatalf("Unable to load root ca cert: %s", err)
		}
		tlsConfig.RootCAs = caCert.PoolContainingCert()
	}
	return tlsConfig
}

type server struct {
	info          *ServerInfo
	masquerades   *verifiedMasqueradeSet
	enproxyConfig *enproxy.Config
	reverseProxy  *httputil.ReverseProxy
}

// buildReverseProxy builds the httputil.ReverseProxy used to proxy requests to
// the server.
func (server *server) buildReverseProxy(shouldDumpHeaders bool) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// do nothing
		},
		Transport: withDumpHeaders(
			shouldDumpHeaders,
			&http.Transport{
				// We disable keepalives because some servers pretend to support
				// keep-alives but close their connections immediately, which
				// causes an error inside ReverseProxy.  This is not an issue
				// for HTTPS because  the browser is responsible for handling
				// the problem, which browsers like Chrome and Firefox already
				// know to do.
				// See https://code.google.com/p/go/issues/detail?id=4677
				DisableKeepAlives: true,
				Dial:              server.dialWithEnproxy,
			}),
		// Set a FlushInterval to prevent overly aggressive buffering of
		// responses, which helps keep memory usage down
		FlushInterval: 250 * time.Millisecond,
	}
}

func (server *server) dialWithEnproxy(network, addr string) (net.Conn, error) {
	conn := &enproxy.Conn{
		Addr:   addr,
		Config: server.getEnproxyConfig(),
	}
	err := conn.Connect()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (server *server) getEnproxyConfig() *enproxy.Config {
	if server.enproxyConfig != nil {
		// Use hardcoded config
		return server.enproxyConfig
	}
	// Build a config on the fly
	return server.buildEnproxyConfig()
}

func (server *server) buildEnproxyConfig() *enproxy.Config {
	return server.info.buildEnproxyConfig(server.nextMasquerade())
}

func (server *server) nextMasquerade() *Masquerade {
	if server.masquerades == nil {
		log.Debugf("No masquerade")
		return nil
	}
	masquerade := server.masquerades.nextVerified()
	log.Debugf("Using masquerade %s", masquerade.Domain)
	return masquerade
}

// withDumpHeaders creates a RoundTripper that uses the supplied RoundTripper
// and that dumps headers is client is so configured.
func withDumpHeaders(shouldDumpHeaders bool, rt http.RoundTripper) http.RoundTripper {
	if !shouldDumpHeaders {
		return rt
	}
	return &headerDumpingRoundTripper{rt}
}

// headerDumpingRoundTripper is an http.RoundTripper that wraps another
// http.RoundTripper and dumps response headers to the log.
type headerDumpingRoundTripper struct {
	orig http.RoundTripper
}

func (rt *headerDumpingRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	proxy.DumpHeaders("Request", &req.Header)
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		proxy.DumpHeaders("Response", &resp.Header)
	}
	return
}
