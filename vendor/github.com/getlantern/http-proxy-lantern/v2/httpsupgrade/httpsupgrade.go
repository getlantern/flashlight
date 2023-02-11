// Package httpsupgrade performs several functions. First, it upgrades uncoming HTTP requests to
// HTTPS for hitting our own services. This is necessary because we need to add special headers
// to those requests, but we cannot do that if they're over TLS. Note that the incoming requests
// are all wrapped in the encryption of the incoming transport, however.
//
// This pacakge also short circuits the normal proxy processing of requests to make direct
// HTTP/2 connections to specific domains such as Lantern internal servers. This saves
// a significant amount of CPU in reducing TLS client handshakes but also makes these
// requests more efficient through the use of persistent connections and HTTP/2's multiplexing
// and reduced headers.
//
// It is worth noting this technique only works over HTTP/2 if the upstream provider, such as
// Cloudflare, supports it. Otherwise it will use regular HTTP and will not benefit to the
// same degree from all of the above benefits, although it will still likely be an improvement
// due to the use of persistent connections.
package httpsupgrade

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy-lantern/v2/common"
	"github.com/getlantern/http-proxy-lantern/v2/domains"
	"github.com/getlantern/proxy/v2/filters"
)

type httpsUpgrade struct {
	httpClient            *http.Client
	log                   golog.Logger
	configServerAuthToken string
}

// NewHTTPSUpgrade creates a new request filter for rewriting requests to HTTPS services..
func NewHTTPSUpgrade(configServerAuthToken string) filters.Filter {
	return &httpsUpgrade{
		httpClient: &http.Client{
			Transport: &http.Transport{
				IdleConnTimeout: 4 * time.Minute,
			},
		},
		log:                   golog.LoggerFor("httpsUpgrade"),
		configServerAuthToken: configServerAuthToken,
	}
}

// Apply implements the filters.Filter interface for HTTP request processing.
func (h *httpsUpgrade) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	if req.Method == "CONNECT" {
		return next(cs, req)
	}
	if cfg := domains.ConfigForRequest(req); cfg.RewriteToHTTPS {
		if cfg.AddConfigServerHeaders {
			h.addConfigServerHeaders(req)
		}
		return h.rewrite(cs, cfg.Host, req)
	}
	return next(cs, req)
}

func (h *httpsUpgrade) addConfigServerHeaders(req *http.Request) {
	if h.configServerAuthToken == "" {
		h.log.Error("No config server auth token?")
		return
	}
	req.Header.Set(common.CfgSvrAuthTokenHeader, h.configServerAuthToken)
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		h.log.Errorf("Unable to split host from '%s': %s", req.RemoteAddr, err)
	} else {
		req.Header.Set(common.CfgSvrClientIPHeader, ip)
	}
}

func (h *httpsUpgrade) rewrite(cs *filters.ConnectionState, host string, r *http.Request) (*http.Response, *filters.ConnectionState, error) {
	req := r.Clone(r.Context())
	req.Host = host + ":443"
	req.URL.Host = req.Host
	req.URL.Scheme = "https"
	h.log.Tracef("Rewrote request with URL %#v to HTTPS", req.URL)
	// Make sure the request stays open.
	req.Close = false

	// The request URI is populated in the request to the proxy but raises an error if populated
	// in outgoing client requests.
	req.RequestURI = ""

	res, err := h.httpClient.Do(req)
	if err != nil {
		h.log.Errorf("Error short circuiting with HTTP/2 with req %#v, %v", req, err)
		return res, cs, err
	}

	// Downgrade the response back to 1.1 to avoid any oddities with clients choking on h2, although
	// no incompatibilities have been observed in the field.
	res.ProtoMajor = 1
	res.ProtoMinor = 1
	res.Proto = "HTTP/1.1"

	// We need to explicitly tell the proxy to close the response, as otherwise particularly responses
	// from the pro server will not be terminated because we don't set the Content-Length in the pro
	// server and instead use Transfer-Encoding chunked. That is advantageous because Heroku will
	// supposedly keep the connection open in that case, but it also means the client does not know
	// how long the response body is, and the chunked encoding somehow doesn't make its way all the way
	// through to the go client, perhaps in part because Cloudflare strips the Transfer-Encoding with
	// the notion that it's the default encoding in the absence of a Content-Length.

	// The short version is that we need to terminate the connection to communicate to clients that the
	// response body is complete, as otherwise Lantern will hang until the TCP connection times out
	// with the idle timer.
	res.Close = true
	return res, cs, err
}
