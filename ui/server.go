package ui

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skratchdot/open-golang/open"

	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/getlantern/systray"
	"github.com/getlantern/tarfs"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/yinbi-server/keystore"
	"github.com/getlantern/yinbi-server/yinbi"
)

// A set of ports that chrome considers restricted
var prohibitedPorts = map[int]bool{
	2049: true, // nfs
	3659: true, // apple-sasl / PasswordServer
	4045: true, // lockd
	6000: true, // X11
	6665: true, // Alternate IRC [Apple addition]
	6666: true, // Alternate IRC [Apple addition]
	6667: true, // Standard IRC [Apple addition]
	6668: true, // Alternate IRC [Apple addition]
	6669: true, // Alternate IRC [Apple addition]
}

func init() {
	// http.FileServer relies on OS to guess mime type, which can be wrong.
	// Override system default for current process.
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".js", "application/javascript")

	unpackUI()
}

var (
	log          = golog.LoggerFor("flashlight.ui")
	fs           *tarfs.FileSystem
	translations = eventual.NewValue()
)

// PathHandler contains a request path pattern and an HTTP handler for that
// pattern.
type PathHandler struct {
	Pattern string
	Handler http.Handler
}

// Listener wraps a net.Listener and provides its
// listen and access addresses
type Listener struct {
	net.Listener
	// The address to listen on, in ":port" form if listen on all interfaces.
	listenAddr string
	// The address client should access. Available only if the server is started.
	accessAddr string
}

// Server serves the local UI.
type Server struct {
	listener *Listener

	// authServerAddr is the address of the Lantern
	// authentication server
	authServerAddr string

	// yinbiClient is a client for the Yinbi API which
	// supports creating accounts and making payments
	yinbiClient *yinbi.Client

	// keystore manages encrypted storage of Yinbi private keys
	keystore *keystore.Keystore

	httpClient *http.Client

	proxy *httputil.ReverseProxy

	externalURL    string
	requestPath    string
	localHTTPToken string
	mux            *http.ServeMux
	onceOpenExtURL sync.Once
	translations   eventual.Value
	standalone     bool
}

// tcpKeepAliveListener is a TCPListener that sets TCP keep-alive
// timeouts on accepted connections.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (l tcpKeepAliveListener) Accept() (net.Conn, error) {
	c, err := l.AcceptTCP()
	if err != nil {
		return nil, err
	}
	c.SetKeepAlive(true)
	c.SetKeepAlivePeriod(5 * time.Minute)
	return c, nil
}

// StartServer creates and starts a new UI server.
// extURL: when supplied, open the URL in addition to the UI address.
// authServerAddr: the Lantern auth server to connect to
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func StartServer(requestedAddr, authServerAddr,
	extURL, localHTTPToken string, standalone bool,
	handlers ...*PathHandler) (*Server, error) {

	server := newServer(extURL, authServerAddr,
		localHTTPToken, standalone)

	for _, h := range handlers {
		server.handle(h.Pattern, h.Handler)
	}

	if err := server.start(requestedAddr); err != nil {
		return nil, err
	}
	return server, nil
}

func newServer(extURL, authServerAddr,
	localHTTPToken string, standalone bool) *Server {
	requestPath := ""
	if localHTTPToken != "" {
		requestPath = "/" + localHTTPToken
	}

	u, err := url.Parse(authServerAddr)
	if err != nil {
		log.Fatal(fmt.Errorf("Bad auth server address: %s", authServerAddr))
	}

	httpClient := &http.Client{
		Timeout:   time.Duration(30 * time.Second),
		Transport: proxied.ChainedThenFronted(),
	}

	s := &Server{
		externalURL:    overrideManotoURL(extURL),
		requestPath:    requestPath,
		httpClient:     httpClient,
		yinbiClient:    newYinbiClient(httpClient),
		keystore:       keystore.New(),
		authServerAddr: authServerAddr,
		proxy:          httputil.NewSingleHostReverseProxy(u),
		localHTTPToken: localHTTPToken,
		translations:   eventual.NewValue(),
		standalone:     standalone,
	}

	s.attachHandlers()

	return s
}

func (s *Server) attachHandlers() {
	mux := http.NewServeMux()

	authHandler := http.HandlerFunc(s.authHandler)
	mux.Handle("/login", s.wrapMiddleware(authHandler))
	mux.Handle("/register", s.wrapMiddleware(authHandler))
	mux.Handle("/payment/new", s.wrapMiddleware(s.sendPaymentHandler()))
	mux.Handle("/user/account/new", s.wrapMiddleware(s.createAccountHandler()))
	mux.Handle("/account/details", s.wrapMiddleware(s.getAccountDetails()))
	mux.Handle("/user/logout", s.wrapMiddleware(s.proxy))
	mux.Handle("/user/mnemonic", s.wrapMiddleware(s.createMnemonic()))
	s.mux = mux
	s.handle("/startup", http.HandlerFunc(s.startupHandler))
	s.handle("/", http.FileServer(fs))
}

// wrapMiddleware takes the given http.Handler and wraps it with
// the auth middleware handlers
func (s *Server) wrapMiddleware(handler http.Handler) http.Handler {
	handler = s.corsHandler(handler)
	return handler
}

// handle directs the underlying server to handle the given pattern at both
// the secure token path and the raw request path. In the case of the raw
// request path, Lantern looks for the token in the Referer HTTP header and
// rejects the request if it's not present.
func (s *Server) handle(pattern string, handler http.Handler) {
	// When the token is included in the request path, we need to strip it in
	// order to serve the UI correctly (i.e. the static UI tarfs FileSystem knows
	// nothing about the request path).
	if s.requestPath != "" {
		// If the request path is empty this will panic on adding the same pattern
		// twice.
		s.mux.Handle(s.requestPath+pattern, util.NoCacheHandler(s.strippingHandler(handler)))
	}

	// In the naked request cast, we need to verify the token is there in the
	// referer header.
	s.mux.Handle(pattern, checkRequestForToken(util.NoCacheHandler(handler), s.localHTTPToken))
}

// This allows a second Lantern running on the system to trigger the existing
// Lantern to show the UI, or at least try to
func (s *Server) startupHandler(resp http.ResponseWriter, req *http.Request) {
	s.ShowRoot("existing", "lantern", nil)
	resp.WriteHeader(http.StatusOK)
}

// strippingHandler removes the secure request path from the URL so that the
// static file server can properly serve it.
func (s *Server) strippingHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("Stripping path from %v", r.URL.Path)
		r.URL.Path = strings.Replace(r.URL.Path, s.requestPath, "", -1)
		h.ServeHTTP(w, r)
	})
}

func closeListener(l net.Listener) {
	err := l.Close()
	if err != nil {
		log.Errorf("Could not close listener: %v", err)
	}
}

// newListener tries to open a connection on the given address.
// If successful, it returns a new Listener
func (s *Server) newListener(address string) (*Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		err = fmt.Errorf("unable to listen at %v: %v", address, err)
		return nil, err
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	if prohibitedPorts[actualPort] {
		err := fmt.Errorf("Client tried to start on prohibited port: %v", actualPort)
		closeListener(listener)
		return nil, err
	}
	log.Debugf("Lantern UI server listening at %v", address)
	listenAddr := address
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		closeListener(listener)
		return nil, err
	}
	listener = tcpKeepAliveListener{listener.(*net.TCPListener)}
	if port == "" || port == "0" {
		// On first run, we pick an arbitrary port, update our listenAddr to
		// reflect the assigned port
		listenAddr = fmt.Sprintf("%v:%v", host, actualPort)
		log.Debugf("rewrote listen address to %v", listenAddr)
	}
	if host == "" {
		host = "localhost"
	}
	accessAddr := net.JoinHostPort(host, strconv.Itoa(actualPort))
	return &Listener{
		listener,
		listenAddr,
		accessAddr,
	}, nil
}

// tryListenCandidates takes a slice of candidate addresses and tries
// to open a connection on one of them.
func (s *Server) tryListenCandidates(candidates []string) (*Listener, error) {
	for _, addr := range candidates {
		l, err := s.newListener(addr)
		if err != nil {
			log.Error(err)
			continue
		}
		return l, nil
	}
	// couldn't start on any of the candidates.
	return nil, errors.New("No address available")
}

// starts server listen at addr in host:port format, or arbitrary local port if
// addr is empty.
func (s *Server) start(requestedAddr string) error {
	listener, err := s.tryListenCandidates(addrCandidates(requestedAddr))
	if err != nil {
		return err
	}
	s.listener = listener
	server := &http.Server{
		Handler:  s.mux,
		ErrorLog: log.AsStdLogger(),
	}
	ch := make(chan error, 1)
	go func() {
		log.Debugf("UI serving at %v", s.listener.Addr())
		err := server.Serve(s.listener)
		ch <- err
	}()

	// to capture the error starting the server
	select {
	case err := <-ch:
		log.Errorf("Error serving: %v", err)
		return err
	case <-time.After(100 * time.Millisecond):
		log.Debugf("UI available at http://%v", s.GetUIAddr())
		return nil
	}
}

// ShowRoot shows the UI at the root level (default).
func (s *Server) ShowRoot(campaign, medium string, st stats.Tracker) {
	s.Show(s.rootURL(), campaign, medium, st)
}

// Show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem. destURL indicates which URL to open.
// When fails to open the browser, it sends a transient alert to the stats
// tracker passed in.
func (s *Server) Show(destURL, campaign, medium string, st stats.Tracker) {
	open := func(u string, t time.Duration) {
		go func() {
			time.Sleep(t)
			if s.standalone {
				systray.ShowAppWindow(u)
			} else {
				if err := open.Run(u); err != nil {
					e := errors.New("Error opening external page to `%v`: %v",
						s.externalURL, err)
					log.Error(e)
					if st != nil {
						st.SetAlert(stats.FAIL_TO_OPEN_BROWSER, e.Error(), true)
					}
				}
			}
		}()
	}
	s.doShow(destURL, campaign, medium, open)
}

// doShow opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func (s *Server) doShow(destURL, campaign, medium string, open func(string, time.Duration)) {
	campaignURL, err := analytics.AddCampaign(destURL, campaign, "", medium)
	var uiURL string
	if err != nil {
		uiURL = destURL
	} else {
		uiURL = campaignURL
	}
	log.Debugf("Opening browser at %v", uiURL)
	open(uiURL, 0*time.Second)

	// This is for opening exernal URLs in a new browser window for
	// partners such as Manoto.
	if s.externalURL != "" {
		s.onceOpenExtURL.Do(func() {
			open(s.externalURL, 4*time.Second)
		})
	}
}

// GetUIAddr returns the current UI address.
func (s *Server) GetUIAddr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.accessAddr
}

// GetListenAddr returns the address the UI server is listening at.
func (s *Server) GetListenAddr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.listenAddr
}

func (s *Server) rootURL() string {
	return s.AddToken("/")
}

func (s *Server) stop() error {
	return s.listener.Close()
}

// AddToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func (s *Server) AddToken(path string) string {
	return "http://" + s.activeDomain() + s.requestPath + path
}

func (s *Server) activeDomain() string {
	return s.GetUIAddr()
}

func checkRequestForToken(h http.Handler, tok string) http.Handler {
	check := func(w http.ResponseWriter, r *http.Request) {
		if HasToken(r, tok) {
			h.ServeHTTP(w, r)
		} else {
			msg := fmt.Sprintf("No token found in. %v", r)
			closeConn(msg, w, r)
		}
	}
	return http.HandlerFunc(check)
}

// HasToken checks for our secure token in the HTTP request.
func HasToken(r *http.Request, tok string) bool {
	if strings.Contains(r.URL.Path, tok) {
		return true
	}
	referer := r.Header.Get("referer")
	if strings.Contains(referer, tok) {
		return true
	}
	return false
}

// closeConn closes the client connection without sending a response.
func closeConn(msg string, w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Error("Response doesn't allow hijacking!")
		return
	}
	connIn, _, err := hj.Hijack()
	if err != nil {
		log.Errorf("Unable to hijack connection: %s", err)
		return
	}
	dumpRequestHeaders(r)
	connIn.Close()
}

func dumpRequestHeaders(r *http.Request) {
	dump, err := httputil.DumpRequest(r, false)
	if err == nil {
		log.Debugf("Request:\n%s", string(dump))
	}
}

func addrCandidates(requested string) []string {
	if strings.HasPrefix(requested, "http://") {
		log.Debugf("Client tried to start at bad address: %v", requested)
		requested = strings.TrimPrefix(requested, "http://")
	}

	if requested != "" {
		return append([]string{requested}, defaultUIAddresses...)
	}
	return defaultUIAddresses
}

func overrideManotoURL(u string) string {
	if strings.HasPrefix(u, "https://www.manoto1.com/") || strings.HasPrefix(u, "https://www.facebook.com/manototv") {
		// Here we make sure to override any old manoto URLs with the latest.
		return "https://www.manototv.com/iran?utm_campaign=manotolantern"
	}
	return u
}

func unpackUI() {
	var err error
	fs, err = tarfs.New(Resources, "")
	if err != nil {
		// Panicking here because this shouldn't happen at runtime unless the
		// resources were incorrectly embedded.
		panic(fmt.Errorf("Unable to open tarfs filesystem: %v", err))
	}
	translations.Set(fs.SubDir("locale/translation"))
}

// Translations returns the translations for a given locale file.
func Translations(filename string) ([]byte, error) {
	log.Tracef("Accessing translations %v", filename)
	tr, ok := translations.Get(30 * time.Second)
	if !ok || tr == nil {
		return nil, fmt.Errorf("Could not get traslation for file name: %v", filename)
	}
	return tr.(*tarfs.FileSystem).Get(filename)
}
