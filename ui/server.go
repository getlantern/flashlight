package ui

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skratchdot/open-golang/open"
	"go.uber.org/zap"

	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/tarfs"
	"github.com/getlantern/zaplog"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/util"
)

func init() {
	// http.FileServer relies on OS to guess mime type, which can be wrong.
	// Override system default for current process.
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".js", "application/javascript")

	unpackUI()
}

var (
	log          = zaplog.LoggerFor("flashlight.ui")
	fs           *tarfs.FileSystem
	translations = eventual.NewValue()
)

// PathHandler contains a request path pattern and an HTTP handler for that
// pattern.
type PathHandler struct {
	Pattern string
	Handler http.Handler
}

// Server serves the local UI.
type Server struct {
	// The address to listen on, in ":port" form if listen on all interfaces.
	listenAddr string
	// The address client should access. Available only if the server is started.
	accessAddr     string
	externalURL    string
	requestPath    string
	localHTTPToken string
	listener       net.Listener
	mux            *http.ServeMux
	onceOpenExtURL sync.Once

	translations eventual.Value
}

// StartServer creates and starts a new UI server.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func StartServer(requestedAddr, extURL, localHTTPToken string,
	handlers ...*PathHandler) (*Server, error) {
	server := newServer(extURL, localHTTPToken)

	for _, h := range handlers {
		server.handle(h.Pattern, h.Handler)
	}

	if err := server.start(requestedAddr); err != nil {
		return nil, err
	}
	return server, nil
}

func newServer(extURL, localHTTPToken string) *Server {
	requestPath := ""
	if localHTTPToken != "" {
		requestPath = "/" + localHTTPToken
	}
	server := &Server{
		externalURL:    overrideManotoURL(extURL),
		requestPath:    requestPath,
		mux:            http.NewServeMux(),
		localHTTPToken: localHTTPToken,
		translations:   eventual.NewValue(),
	}

	server.attachHandlers()
	return server
}

func (s *Server) attachHandlers() {
	// This allows a second Lantern running on the system to trigger the existing
	// Lantern to show the UI, or at least try to
	startupHandler := func(resp http.ResponseWriter, req *http.Request) {
		s.ShowRoot("existing", "lantern", nil)
		resp.WriteHeader(http.StatusOK)
	}

	s.handle("/startup", http.HandlerFunc(startupHandler))
	s.handle("/", http.FileServer(fs))
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

// strippingHandler removes the secure request path from the URL so that the
// static file server can properly serve it.
func (s *Server) strippingHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Infof("Stripping path from %v", r.URL.Path)
		r.URL.Path = strings.Replace(r.URL.Path, s.requestPath, "", -1)
		h.ServeHTTP(w, r)
	})
}

// starts server listen at addr in host:port format, or arbitrary local port if
// addr is empty.
func (s *Server) start(requestedAddr string) error {
	var listenErr error
	for _, addr := range addrCandidates(requestedAddr) {
		log.Infof("Lantern UI server start listening at %v", addr)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			listenErr = fmt.Errorf("unable to listen at %v: %v", addr, err)
			log.Info(listenErr)
			continue
		}
		s.listenAddr = addr
		s.listener = l
		goto serve
	}
	// couldn't start on any of the candidates.
	return listenErr

serve:
	actualPort := s.listener.Addr().(*net.TCPAddr).Port
	host, port, err := net.SplitHostPort(s.listenAddr)
	if err != nil {
		panic("impossible")
	}
	if port == "" || port == "0" {
		// On first run, we pick an arbitrary port, update our listenAddr to
		// reflect the assigned port
		s.listenAddr = fmt.Sprintf("%v:%v", host, actualPort)
		log.Infof("rewrote listen address to %v", s.listenAddr)
	}
	if host == "" {
		host = "localhost"
	}
	s.accessAddr = net.JoinHostPort(host, strconv.Itoa(actualPort))

	server := &http.Server{
		Handler:  s.mux,
		ErrorLog: zap.NewStdLog(log.Desugar()),
	}
	ch := make(chan error, 1)
	go func() {
		log.Infof("UI serving at %v", s.listener.Addr())
		err := server.Serve(s.listener)
		ch <- err
	}()

	// to capture the error starting the server
	select {
	case err := <-ch:
		log.Errorf("Error serving: %v", err)
		return err
	case <-time.After(100 * time.Millisecond):
		log.Infof("UI available at http://%v", s.accessAddr)
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
			err := open.Run(u)
			if err != nil {
				e := errors.New("Error opening external page to `%v`: %v",
					s.externalURL, err)
				log.Error(e)
				if st != nil {
					st.SetAlert(stats.FAIL_TO_OPEN_BROWSER, e.Error(), true)
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
	log.Infof("Opening browser at %v", uiURL)
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
	return "http://" + s.activeDomain() + s.requestPath + path
}

func (s *Server) activeDomain() string {
	return s.accessAddr
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
		log.Infof("Request:\n%s", string(dump))
	}
}

func addrCandidates(requested string) []string {
	if strings.HasPrefix(requested, "http://") {
		log.Errorf("Client tried to start at bad address: %v", requested)
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
	translations.Set(fs.SubDir("locale"))
}

// Translations returns the translations for a given locale file.
func Translations(filename string) ([]byte, error) {
	log.Debugf("Accessing translations %v", filename)
	tr, ok := translations.Get(30 * time.Second)
	if !ok || tr == nil {
		return nil, fmt.Errorf("Could not get traslation for file name: %v", filename)
	}
	return tr.(*tarfs.FileSystem).Get(filename)
}
