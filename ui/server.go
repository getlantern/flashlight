package ui

import (
	"github.com/getlantern/flashlight/util"
	"net"
	"net/http"
	"net/url"
)

type ServeMux struct {
	*http.ServeMux
}

func NewServeMux() *ServeMux {
	return &ServeMux{ServeMux: http.NewServeMux()}
}

func (s *ServeMux) Handle(pattern string, handler http.Handler) {
	applyMiddleware := func(h http.Handler) http.Handler {
		return checkOrigin(util.NoCacheHandler(h))
	}
	s.ServeMux.Handle(pattern, applyMiddleware(handler))
}

func checkOrigin(h http.Handler) http.Handler {
	check := func(w http.ResponseWriter, r *http.Request) {
		var clientAddr string

		referer := r.Header.Get("Referer")
		if referer != "" {
			clientAddr = referer
		}

		origin := r.Header.Get("Origin")
		if origin != "" {
			clientAddr = origin
		}

		tokenMatch := false

		if clientAddr == "" {
			switch r.URL.Path {
			case "/": // Whitelist skips any further checks.
				h.ServeHTTP(w, r)
				return
			default:
				r.ParseForm()
				token := r.Form.Get("token")
				if token == sessionToken {
					clientAddr = uiaddr // Bypass further checks if the token is legit.
					tokenMatch = true
				} else {
					log.Errorf("Access to %v was denied because no valid Origin or Referer headers were provided.", r.URL)
					return
				}
			}
		}

		expectedURL, err := url.Parse(uiaddr)
		if err != nil {
			log.Fatalf("Could not parse own uiaddr: %v", err)
		}

		originURL, err := url.Parse(clientAddr)
		if err != nil {
			log.Errorf("Could not parse client addr %v", clientAddr)
			return
		}

		if strictOriginCheck && !tokenMatch {
			if allowRemoteClients {
				// At least check if same port.
				_, originPort, _ := net.SplitHostPort(originURL.Host)
				_, expectedPort, _ := net.SplitHostPort(expectedURL.Host)
				if originPort != expectedPort {
					log.Errorf("Expecting clients connect on port: %s, but got: %s", expectedPort, originPort)
					return
				}
			} else {
				if GetPreferredUIAddr() != "http://"+originURL.Host {
					log.Errorf("Origin was '%v' but expecting: '%v'", "http://"+originURL.Host, GetPreferredUIAddr())
					log.Errorf("Call to %v", r.URL)
					return
				}
			}
		}

		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(check)
}
