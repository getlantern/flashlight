package ui

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/flashlight/util"
	"github.com/skratchdot/open-golang/open"
)

type Server struct {
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

// NewServer creates a new UI server listen at addr in host:port format, or
// arbitrary local port if addr is empty.
// allInterfaces: when true, server will listen on all local interfaces,
// regardless of what the addr parameter is.
// extURL: when supplied, open the URL in addition to the UI address.
// localHTTPToken: if set, close client connection directly if the request
// doesn't bring the token in query parameters nor have the same origin.
func NewServer(addr string, allInterfaces bool, extURL, localHTTPToken string) *Server {
	addr = normalizeAddr(addr)
	if allInterfaces {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			log.Errorf("invalid address %v", addr)
			port = "0"
		}
		addr = ":" + port
	}

	return &Server{
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
	} else {
		return u
	}
}

// Handle let the Server to handle the pattern using handler.
func (s *Server) Handle(pattern string, handler http.Handler) {
	log.Debugf("Adding handler for %v", pattern)
	s.mux.Handle(pattern,
		checkOrigin(
			util.NoCacheHandler(handler),
			s.localHTTPToken,
			s.listenAddr))
}

func (s *Server) Start() error {
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
	if port == "0" {
		// On first run, we pick an arbitrary port, update our listenAddr to reflect
		// the assigned port
		s.listenAddr = fmt.Sprintf("%v:%v", host, actualPort)
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
	ch := make(chan error)
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

// Show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func (s *Server) Show() {
	go func() {
		uiURL := fmt.Sprintf("http://%s/?1", s.accessAddr)
		log.Debugf("Opening browser at %v", uiURL)
		err := open.Run(uiURL)
		if err != nil {
			log.Errorf("Error opening page to `%v`: %v", uiURL, err)
		}

		// This is for opening exernal URLs in a new browser window for
		// partners such as Manoto.
		if s.externalURL != "" {
			s.onceOpenExtURL.Do(func() {
				time.Sleep(4 * time.Second)
				err := open.Run(s.externalURL)
				if err != nil {
					log.Errorf("Error opening external page to `%v`: %v", s.externalURL, err)
				}
			})
		}
	}()
}

// GetUIAddr returns the current UI address.
func (s *Server) GetUIAddr() string {
	return s.accessAddr
}

func (s *Server) Stop() error {
	return s.listener.Close()
}

// AddToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func (s *Server) AddToken(in string) string {
	return util.SetURLParam("http://"+path.Join(s.accessAddr, in), "token", s.localHTTPToken)
}

func checkOrigin(h http.Handler, localHTTPToken, listenAddr string) http.Handler {
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
				if token == localHTTPToken {
					tokenMatch = true
				} else if token != "" {
					log.Errorf("Token '%v' did not match the expected '%v'", token, localHTTPToken)
				} else {
					log.Errorf("Access to %v was denied because no valid Origin or Referer headers were provided.", r.URL)
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
			if !strings.HasSuffix(originHost, listenAddr) {
				log.Errorf("Origin was '%v' but expecting: '%v'", originHost, listenAddr)
				return
			}
		}

		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(check)
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
