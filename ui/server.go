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
				if token == sessionToken {
					tokenMatch = true
				} else {
					log.Errorf("Access to %v was denied because no valid Origin or Referer headers were provided.", r.URL)
					return
				}
			}
		}

		if strictOriginCheck && !tokenMatch {
			var originHost string
			if originURL, err := url.Parse(clientURL); err != nil {
				log.Errorf("Could not parse client URL %v", clientURL)
				return
			} else {
				originHost = originURL.Host
			}

			if allowRemoteClients {
				// At least check if same port.
				_, originPort, _ := net.SplitHostPort(originHost)
				_, expectedPort, _ := net.SplitHostPort(uiaddr)
				if originPort != expectedPort {
					log.Errorf("Expecting clients connect on port: %s, but got: %s", expectedPort, originPort)
					return
				}
			} else {
				if GetPreferredUIAddr() != originHost {
					log.Errorf("Origin was '%v' but expecting: '%v'", originHost, GetPreferredUIAddr())
					return
				}
			}
		}

		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(check)
}
