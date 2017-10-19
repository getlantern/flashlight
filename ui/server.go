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

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
	"github.com/getlantern/tarfs"
	"github.com/skratchdot/open-golang/open"
)

func init() {
	// http.FileServer relies on OS to guess mime type, which can be wrong.
	// Override system default for current process.
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".js", "application/javascript")
}

var (
	log          = golog.LoggerFor("flashlight.ui")
	fs           *tarfs.FileSystem
	translations = eventual.NewValue()
)

//PathHandler contains a request path pattern and an HTTP handler for that
//pattern.
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

	// The domain to serve the UI on - can be anything really.
	uiDomain     string
	useUIDomain  func() bool
	translations eventual.Value
}

// StartServer creates and starts a new UI server.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func StartServer(requestedAddr, extURL, localHTTPToken, uiDomain string, useUIDomain func() bool,
	handlers ...*PathHandler) (*Server, error) {
	server := newServer(extURL, localHTTPToken, uiDomain, useUIDomain)

	for _, h := range handlers {
		server.handle(h.Pattern, h.Handler)
	}

	if err := server.start(requestedAddr); err != nil {
		return nil, err
	}
	return server, nil
}

// StartServer creates and starts a new UI server.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func newServer(extURL, localHTTPToken, uiDomain string, useUIDomain func() bool) *Server {
	requestPath := ""
	if localHTTPToken != "" {
		requestPath = "/" + localHTTPToken
	}
	server := &Server{
		externalURL:    overrideManotoURL(extURL),
		requestPath:    requestPath,
		mux:            http.NewServeMux(),
		uiDomain:       uiDomain,
		useUIDomain:    useUIDomain,
		localHTTPToken: localHTTPToken,
		translations:   eventual.NewValue(),
	}
	server.unpackUI()

	server.attachHandlers()
	return server
}

func (s *Server) attachHandlers() {
	// This allows a second Lantern running on the system to trigger the existing
	// Lantern to show the UI, or at least try to
	startupHandler := func(resp http.ResponseWriter, req *http.Request) {
		s.ShowRoot("existing", "lantern")
		resp.WriteHeader(http.StatusOK)
	}

	s.handle("/startup", s.strippingHandler(http.HandlerFunc(startupHandler)))
	s.handle("/", s.strippingHandler(http.FileServer(fs)))
}

// handle directls the underlying server to handle the given pattern at both
// the secure token path and the raw request path. In the case of the raw
// request path, Lantern looks for the token in the Referer HTTP header and
// reject the request if it's not present.
func (s *Server) handle(pattern string, handler http.Handler) {
	base := s.checkRequestPath(util.NoCacheHandler(handler))
	s.mux.Handle(s.requestPath+pattern, base)
	s.mux.Handle(pattern, base)
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

// starts server listen at addr in host:port format, or arbitrary local port if
// addr is empty.
func (s *Server) start(requestedAddr string) error {
	var listenErr error
	for _, addr := range addrCandidates(requestedAddr) {
		log.Debugf("Lantern UI server start listening at %v", addr)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			listenErr = fmt.Errorf("unable to listen at %v: %v", addr, err)
			log.Debug(listenErr)
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
func (s *Server) ShowRoot(campaign, medium string) {
	s.Show(s.rootURL(), campaign, medium)
}

// Show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem. destURL indicates which URL to open.
func (s *Server) Show(destURL, campaign, medium string) {
	open := func(u string, t time.Duration) {
		go func() {
			time.Sleep(t)
			err := open.Run(u)
			if err != nil {
				log.Errorf("Error opening external page to `%v`: %v", s.externalURL, err)
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
	if s.useUIDomain() {
		return s.uiDomain
	}
	return s.accessAddr
}

func (s *Server) checkRequestPath(h http.Handler) http.Handler {
	check := func(w http.ResponseWriter, r *http.Request) {
		if s.rejectRequest(r) {
			msg := fmt.Sprintf("Access denied because the request path was wrong. Expected\n'%v'\nnot:\n%v",
				s.requestPath, r.URL.Path)
			s.notFound(msg, w, r)
		} else {
			h.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(check)
}

func (s *Server) rejectRequest(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, s.requestPath) {
		referer := r.Header.Get("referer")
		if !strings.Contains(referer, s.localHTTPToken) {
			return true
		}
	}
	return false
}

// notFound returns a 403 forbidden response to the client while also dumping
// headers and logs for debugging.
func (s *Server) notFound(msg string, w http.ResponseWriter, r *http.Request) {
	log.Error(msg)
	s.dumpRequestHeaders(r)
	// Return forbidden but do not reveal any details in the body.
	http.Error(w, "", http.StatusNotFound)
}

func (s *Server) dumpRequestHeaders(r *http.Request) {
	dump, err := httputil.DumpRequest(r, false)
	if err == nil {
		log.Debugf("Request:\n%s", string(dump))
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

func (s *Server) unpackUI() {
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
	log.Tracef("Accessing translations %v", filename)
	tr, ok := translations.Get(30 * time.Second)
	if !ok || tr == nil {
		return nil, fmt.Errorf("Could not get traslation for file name: %v", filename)
	}
	return tr.(*tarfs.FileSystem).Get(filename)
}
