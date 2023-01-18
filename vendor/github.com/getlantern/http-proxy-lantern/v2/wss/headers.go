package wss

import (
	"net"
	"net/http"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy-lantern/v2/domains"
	"github.com/getlantern/netx"
	"github.com/getlantern/proxy/v2/filters"
	"github.com/getlantern/tinywss"
)

var (
	log = golog.LoggerFor("wss")
	// these headers are replicated from the inital http upgrade request
	// to certain subrequests on a wss connection.
	headerWhitelist = []string{
		"CloudFront-Viewer-Country",
	}
)

type middleware struct{}

func NewMiddleware() *middleware {
	return &middleware{}
}

func (m *middleware) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	m.apply(cs, req)
	return next(cs, req)
}

func (m *middleware) apply(cs *filters.ConnectionState, req *http.Request) {

	// carries through certain headers on authorized connections from CDNs
	// for domains that are configured to receive client ip information.
	// the connecting ip is a CDN edge server, so the req.RemoteAddr does
	// not reflect the correct client ip.

	cfg := domains.ConfigForRequest(req)
	if !(cfg.AddConfigServerHeaders) {
		return
	}

	conn := cs.Downstream()
	netx.WalkWrapped(conn, func(conn net.Conn) bool {
		if t, ok := conn.(*tinywss.WsConn); ok {
			upHdr := t.UpgradeHeaders()
			// XXX use an auth token here to prove it's a CDN
			for _, header := range headerWhitelist {
				if val := upHdr.Get(header); val != "" {
					req.Header.Set(header, val)
					log.Tracef("WSS: copied header %s (%s)", header, val)
				} else {
					log.Tracef("WSS: header %s was not present!", header)
				}
			}
			return false
		}

		// Keep looking
		return true
	})
}
