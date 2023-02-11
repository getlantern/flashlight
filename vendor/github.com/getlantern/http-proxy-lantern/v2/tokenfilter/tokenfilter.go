package tokenfilter

import (
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/v2/filters"

	"github.com/getlantern/http-proxy-lantern/v2/common"
	"github.com/getlantern/http-proxy-lantern/v2/instrument"
	"github.com/getlantern/http-proxy-lantern/v2/mimic"
)

var log = golog.LoggerFor("tokenfilter")

type tokenFilter struct {
	token      string
	instrument instrument.Instrument
}

func New(token string, instrument instrument.Instrument) filters.Filter {
	return &tokenFilter{
		token:      token,
		instrument: instrument,
	}
}

func (f *tokenFilter) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("Token Filter Middleware received request:\n%s", reqStr)
	}

	if f.token == "" {
		log.Trace("Not checking token")
		return next(cs, req)
	}

	tokens := req.Header[common.TokenHeader]
	if tokens == nil || len(tokens) == 0 || tokens[0] == "" {
		log.Errorf("No token provided, mimicking apache")
		f.instrument.Mimic(true)
		return mimicApache(cs, req)
	}
	tokenMatched := false
	for _, candidate := range tokens {
		if candidate == f.token {
			tokenMatched = true
			break
		}
	}
	if tokenMatched {
		req.Header.Del(common.TokenHeader)
		log.Tracef("Allowing connection from %v to %v", req.RemoteAddr, req.Host)
		f.instrument.Mimic(false)
		return next(cs, req)
	}
	log.Errorf("Mismatched token(s) %v, mimicking apache", strings.Join(tokens, ","))
	f.instrument.Mimic(true)
	return mimicApache(cs, req)
}

func mimicApache(cs *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
	conn := cs.Downstream()
	mimic.Apache(conn, req)
	conn.Close()
	return nil, cs, nil
}
