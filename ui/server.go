package ui

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/util"
	"github.com/skratchdot/open-golang/open"
)

type server struct {
	// The address to listen on, in ":port" form if listen on all interfaces.
	listenAddr string
	// The address client should access. Available only if the server is started.
	accessAddr     string
	externalURL    string
	localHTTPToken string
	listener       net.Listener
	mux            *http.ServeMux
	onceOpenExtURL sync.Once
}

// newServer creates a new UI server listen at addr in host:port format, or
// arbitrary local port if addr is empty.
// allInterfaces: when true, server will listen on all local interfaces,
// regardless of what the addr parameter is.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func newServer(addr string, allInterfaces bool, extURL, localHTTPToken string) *server {
	addr = normalizeAddr(addr)
	if allInterfaces {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			log.Errorf("invalid address %v", addr)
			port = "0"
		}
		addr = ":" + port
	}

	return &server{
		listenAddr:     addr,
		externalURL:    overrideManotoURL(extURL),
		localHTTPToken: localHTTPToken,
		mux:            http.NewServeMux(),
	}
}

func overrideManotoURL(u string) string {
	if strings.HasPrefix(u, "https://www.manoto1.com/") || strings.HasPrefix(u, "https://www.facebook.com/manototv") {
		// Here we make sure to override any old manoto URLs with the latest.
		return "https://www.manototv.com/iran?utm_campaign=manotolantern"
	}
	return u
}

// Handle let the Server to handle the pattern using handler.
func (s *server) Handle(pattern string, handler http.Handler) {
	log.Debugf("Adding handler for %v", pattern)
	s.mux.Handle(pattern,
		s.checkOrigin(util.NoCacheHandler(handler)))
}

func (s *server) start() error {
	log.Debugf("Lantern UI server start listening at %v", s.listenAddr)
	l, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("Unable to listen at %v. Error is: %v", s.listenAddr, err)
	}

	actualPort := l.Addr().(*net.TCPAddr).Port

	host, port, err := net.SplitHostPort(s.listenAddr)
	if err != nil {
		log.Errorf("invalid address %v", s.listenAddr)
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
	s.listener = l

	server := &http.Server{
		Handler:  s.mux,
		ErrorLog: log.AsStdLogger(),
	}
	ch := make(chan error, 1)
	go func() {
		log.Debugf("UI serving at %v", l.Addr())
		err := server.Serve(l)
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

// show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func (s *server) show(campaign, content, medium string) {
	open := func(u string, t time.Duration) {
		go func() {
			time.Sleep(t)
			err := open.Run(u)
			if err != nil {
				log.Errorf("Error opening external page to `%v`: %v", s.externalURL, err)
			}
		}()
	}
	s.doShow(campaign, content, medium, open)
}

// doShow opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func (s *server) doShow(campaign, content, medium string, open func(string, time.Duration)) {
	tempURL := fmt.Sprintf("http://%s/", s.accessAddr)
	campaignURL, err := analytics.AddCampaign(tempURL, campaign, content, medium)
	var uiURL string
	if err != nil {
		uiURL = tempURL
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

// getUIAddr returns the current UI address.
func (s *server) getUIAddr() string {
	return s.accessAddr
}

func (s *server) stop() error {
	return s.listener.Close()
}

// addToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func (s *server) addToken(in string) string {
	return util.SetURLParam("http://"+path.Join(s.accessAddr, in), "token", s.localHTTPToken)
}

func (s *server) checkOrigin(h http.Handler) http.Handler {
	check := func(w http.ResponseWriter, r *http.Request) {
		var clientURL string

		referer := r.Header.Get("Referer")
		if referer != "" {
			clientURL = referer
		}

		origin := r.Header.Get("Origin")
		if origin != "" {
			clientURL = origin
		}

		tokenMatch := false

		if clientURL == "" {
			switch r.URL.Path {
			case "/": // Whitelist skips any further checks.
				h.ServeHTTP(w, r)
				return
			default:
				r.ParseForm()
				token := r.Form.Get("token")
				if token == s.localHTTPToken {
					tokenMatch = true
				} else if token != "" {
					msg := fmt.Sprintf("Token '%v' did not match the expected '%v...'", token, s.localHTTPToken)
					s.forbidden(msg, w, r)
					return
				} else {
					msg := fmt.Sprintf("Access to %v was denied because no valid Origin or Referer headers were provided.", r.URL)
					s.forbidden(msg, w, r)
					return
				}
			}
		}

		if strictOriginCheck && !tokenMatch {
			originURL, err := url.Parse(clientURL)
			if err != nil {
				log.Errorf("Could not parse client URL %v", clientURL)
				return
			}
			originHost := originURL.Host
			// when Lantern is listening on all interfaces, e.g., allow remote
			// connections, listenAddr is in ":port" form. Using HasSuffix
			if !strings.HasSuffix(originHost, s.listenAddr) {
				msg := fmt.Sprintf("Origin was '%v' but expecting: '%v'", originHost, s.listenAddr)
				s.forbidden(msg, w, r)
				return
			}
		}

		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(check)
}

// forbidden returns a 403 forbidden response to the client while also dumping
// headers and logs for debugging.
func (s *server) forbidden(msg string, w http.ResponseWriter, r *http.Request) {
	log.Error(msg)
	s.dumpRequestHeaders(r)
	// Return forbidden but do not reveal any details in the body.
	http.Error(w, "", http.StatusForbidden)
}

func (s *server) dumpRequestHeaders(r *http.Request) {
	dump, err := httputil.DumpRequest(r, false)
	if err == nil {
		log.Debugf("Request:\n", string(dump))
	}
}

func normalizeAddr(addr string) string {
	if addr == "" {
		return defaultUIAddress
	} else if strings.HasPrefix(addr, "http://") {
		log.Errorf("Client tried to start at bad address: %v", addr)
		return strings.TrimPrefix(addr, "http://")
	}
	return addr
}
