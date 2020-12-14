package ui

import (
	"bytes"
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
	"unicode"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"

	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/getlantern/tarfs"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ui/auth"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/flashlight/ui/yinbi"
	"github.com/getlantern/flashlight/util"
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

// Server serves the local UI.
type Server struct {
	// The address to listen on, in ":port" form if listen on all interfaces.
	listenAddr string
	// The address client should access. Available only if the server is started.
	accessAddr  string
	externalURL string
	// Prefixed to handler patterns when the http token is expected in the request path.
	httpTokenRequestPathPrefix string
	localHTTPToken             string
	listener                   net.Listener
	mux                        *http.ServeMux
	onceOpenExtURL             sync.Once

	translations eventual.Value
	standalone   bool
}

// PathHandler contains a request path pattern and an HTTP handler for that
// pattern.
type PathHandler struct {
	Pattern string
	Handler http.Handler
}

// ServerParams specifies the parameters to use
// when creating new UI server
type ServerParams struct {
	ExtURL          string
	AuthServerAddr  string
	YinbiServerAddr string
	AppName         string
	LocalHTTPToken  string
	RequestedAddr   string
	SkipTokenCheck  bool
	Standalone      bool
	HTTPClient      *http.Client
	Handlers        []PathHandler
}

// Middleware
type Middleware func(http.HandlerFunc) http.HandlerFunc

// StartServer creates and starts a new UI server.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func StartServer(params ServerParams) (*Server, error) {
	server := newServer(params)

	if err := server.start(params.RequestedAddr); err != nil {
		return nil, err
	}
	return server, nil
}

func newServer(params ServerParams) *Server {
	localHTTPToken := params.LocalHTTPToken
	server := &Server{
		externalURL: overrideManotoURL(params.ExtURL),
		httpTokenRequestPathPrefix: func() string {
			if localHTTPToken == "" {
				return ""
			}
			return "/" + localHTTPToken
		}(),
		mux:            http.NewServeMux(),
		localHTTPToken: localHTTPToken,
		translations:   eventual.NewValue(),
		standalone:     params.Standalone,
	}

	server.attachHandlers(params)

	return server
}

func (s *Server) attachHandlers(params ServerParams) {

	// This allows a second Lantern running on the system to trigger the existing
	// Lantern to show the UI, or at least try to
	startupHandler := func(resp http.ResponseWriter, req *http.Request) {
		s.ShowRoot("existing", "lantern", nil)
		resp.WriteHeader(http.StatusOK)
	}

	httpClient := createHTTPClient()

	apiParams := api.NewAPIParams(params.AppName, params.AuthServerAddr,
		params.YinbiServerAddr, httpClient)

	authHandler := auth.New(apiParams)

	handlers := []handler.UIHandler{
		authHandler,
		yinbi.NewWithAuth(apiParams, authHandler),
	}

	// configure UI handlers with routes setup internally
	for _, h := range handlers {
		prefix := h.GetPathPrefix()
		s.Handle(prefix, http.StripPrefix(prefix, h.ConfigureRoutes()))
	}

	// configure routes passed with server params
	for _, h := range params.Handlers {
		s.Handle(h.Pattern, h.Handler)
	}

	s.Handle("/startup", util.NoCache(http.HandlerFunc(startupHandler)))
	s.Handle("/", util.NoCache(http.FileServer(fs)))
}

// createHTTPClient creates a chained-then-fronted configured HTTP client
// to be used by multiple UI handlers
func createHTTPClient() *http.Client {
	rt := proxied.ChainedThenFronted()
	rt.SetMasqueradeTimeout(30 * time.Second)
	return &http.Client{
		Transport: rt,
	}
}

// Handle directs the underlying server to handle the given pattern at both
// the secure token path and the raw request path. In the case of the raw
// request path, Lantern looks for the token in the Referer HTTP header and
// rejects the request if it's not present.
func (s *Server) Handle(pattern string, handler http.Handler) {
	// When the token is included in the request path, we need to strip it in
	// order to serve the UI correctly (i.e. the static UI tarfs FileSystem knows
	// nothing about the request path).
	if s.httpTokenRequestPathPrefix != "" {
		// If the request path is empty this would panic on adding the same pattern
		// twice.
		s.mux.Handle(s.httpTokenRequestPathPrefix+pattern, s.strippingHandler(handler))
	}

	// In the naked request cast, we need to verify the token is there in the
	// referer header.
	s.mux.Handle(pattern, checkRequestForToken(handler, s.localHTTPToken))

}

// strippingHandler removes the secure request path from the URL so that the
// static file server can properly serve it.
func (s *Server) strippingHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripped := strings.Replace(r.URL.Path, s.httpTokenRequestPathPrefix, "", -1)
		log.Debugf("changing path from %q to %q", r.URL.Path, stripped)
		r.URL.Path = stripped
		h.ServeHTTP(w, r)
	})
}

// takes a slice of address candidates and listens on the first
// acceptable one returning the listener and address
func listen(candidates []string) (net.Listener, string, error) {
	var listenErr error
	for _, addr := range candidates {
		for {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				listenErr = fmt.Errorf("unable to listen at %v: %v", addr, err)
				log.Debug(listenErr)
				break // move to the next candidate
			}
			actualPort := listener.Addr().(*net.TCPAddr).Port
			if !prohibitedPorts[actualPort] {
				return listener, addr, nil
			} else {
				listenErr = fmt.Errorf("Client tried to start on prohibited port: %v", actualPort)
				log.Debug(listenErr)
				closeErr := listener.Close()
				if closeErr != nil {
					log.Errorf("Could not close listener on prohibited port %v: %v", actualPort, closeErr)
				}
				_, port, err := net.SplitHostPort(addr)
				if err != nil {
					listenErr = fmt.Errorf("error parsing addr %v: %v", addr, err)
					log.Debug(listenErr)
					break
				}
				if port == "" || port == "0" {
					continue
				} else {
					break
				}
			}
		}
	}
	return nil, "", listenErr
}

// starts server listen at addr in host:port format, or arbitrary local port if
// addr is empty.
func (s *Server) start(requestedAddr string) error {
	listener, addr, err := listen(addrCandidates(requestedAddr))
	if err != nil {
		return err
	}
	s.listenAddr = addr
	s.listener = listener
	actualPort := s.listener.Addr().(*net.TCPAddr).Port
	host, port, err := net.SplitHostPort(s.listenAddr)
	if err != nil {
		panic("impossible")
	}
	if port == "" || port == "0" {
		// On first run, we pick an arbitrary port, update our listenAddr to
		// reflect the assigned port
		s.listenAddr = fmt.Sprintf("%v:%v", host, actualPort)
		log.Debugf("rewrote listen address to %v", s.listenAddr)
	}
	if host == "" {
		host = "localhost"
	}
	s.accessAddr = net.JoinHostPort(host, strconv.Itoa(actualPort))

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
		log.Debugf("UI available at http://%v", s.accessAddr)
		return nil
	}
}

// ShowRoot shows the UI at the root level (default).
func (s *Server) ShowRoot(campaign, medium string, st stats.Tracker) {
	s.Show(s.rootURL(), strings.ToLower(campaign), strings.ToLower(medium), st)
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
				// systray.ShowAppWindow(u)
				log.Error("Standalone mode currently not supported, opening in system browser")
				// TODO: re-enable standalone mode when systray library has been stabilized
			}
			// } else {
			if err := open.Run(u); err != nil {
				e := errors.New("Error opening external page to `%v`: %v",
					s.externalURL, err)
				log.Error(e)
				if st != nil {
					st.SetAlert(stats.FAIL_TO_OPEN_BROWSER, e.Error(), true)
				}
			}
			// }
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
	campaignURL, err := AddCampaign(destURL, campaign, "", medium)
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
	return s.accessAddr
}

// GetListenAddr returns the address the UI server is listening at.
func (s *Server) GetListenAddr() string {
	return s.listenAddr
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
	return "http://" + s.activeDomain() + s.httpTokenRequestPathPrefix + path
}

func (s *Server) activeDomain() string {
	return s.accessAddr
}

func checkRequestForToken(h http.Handler, tok string) http.Handler {
	check := func(w http.ResponseWriter, r *http.Request) {
		if HasToken(r, tok) {
			h.ServeHTTP(w, r)
		} else {
			b, err := httputil.DumpRequest(r, false)
			if err != nil {
				log.Errorf("error dumping request: %v", err)
			}
			log.Debugf("no token in request:\n%s", bytes.TrimRightFunc(b, unicode.IsSpace))
			err = closeConn(w)
			if err != nil {
				log.Errorf("error closing request conn: %v", err)
				http.Error(w, "token not found", http.StatusBadRequest)
			}
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
func closeConn(w http.ResponseWriter) error {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("response doesn't implement hijacker")
	}
	connIn, _, err := hj.Hijack()
	if err != nil {
		return xerrors.Errorf("hijacking response: %w", err)
	}
	return connIn.Close()
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

// AddCampaign adds Google Analytics campaign tracking to a URL and returns
// that URL.
func AddCampaign(urlStr, campaign, content, medium string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		log.Errorf("Could not parse click URL: %v", err)
		return "", err
	}

	q := u.Query()
	q.Set("utm_source", common.Platform)
	q.Set("utm_medium", medium)
	q.Set("utm_campaign", campaign)
	q.Set("utm_content", content)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
