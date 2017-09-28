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
	proxy          *httputil.ReverseProxy
}

// newServer creates a new UI server.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func newServer(extURL, localHTTPToken string) *server {
	return &server{
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

// starts server listen at addr in host:port format, or arbitrary local port if
// addr is empty.
func (s *server) start(requestedAddr string) error {
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

// show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func (s *server) show(campaign, medium string) {
	open := func(u string, t time.Duration) {
		go func() {
			time.Sleep(t)
			err := open.Run(u)
			if err != nil {
				log.Errorf("Error opening external page to `%v`: %v", s.externalURL, err)
			}
		}()
	}
	s.doShow(campaign, medium, open)
}

// doShow opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func (s *server) doShow(campaign, medium string, open func(string, time.Duration)) {
	tempURL := fmt.Sprintf("http://search.lantern.io?token=%s", s.localHTTPToken)
	campaignURL, err := analytics.AddCampaign(tempURL, campaign, "", medium)
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

func (s *server) setRequestToken(req *http.Request) {
	u := req.URL
	q := u.Query()
	q.Set("token", s.localHTTPToken)
	u.RawQuery = q.Encode()
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

		token := r.URL.Query().Get("token")
		tokenMatch := token == s.localHTTPToken

		if clientURL == "" {
			switch {
			case r.URL.Path == "/":
				// we don't bother with additional checks for root URL
				h.ServeHTTP(w, r)
				return
			case tokenMatch:
				h.ServeHTTP(w, r)
				return
			case token != "":
				msg := fmt.Sprintf("Token '%v' did not match the expected '%v...'", token, s.localHTTPToken)
				s.forbidden(msg, w, r)
				return
			default:
				msg := fmt.Sprintf("Access to %v was denied because no valid Origin or Referer headers were provided.", r.URL)
				s.forbidden(msg, w, r)
				return
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
