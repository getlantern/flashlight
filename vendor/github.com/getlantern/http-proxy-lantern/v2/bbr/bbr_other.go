// +build !linux

package bbr

import (
	"net"
	"net/http"

	"github.com/getlantern/proxy/v2/filters"
)

// noopMiddleware is used on non-linux platforms where BBR is unavailable.
type noopMiddleware struct{}

func New() Middleware {
	return &noopMiddleware{}
}

func (bm *noopMiddleware) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	return next(cs, req)
}

func (bm *noopMiddleware) AddMetrics(_ *filters.ConnectionState, _ *http.Request, _ *http.Response) {
}

func (bm *noopMiddleware) Wrap(l net.Listener) net.Listener {
	return l
}

func (bm *noopMiddleware) ABE(_ *filters.ConnectionState) float64 {
	return 0
}

func (bm *noopMiddleware) ProbeUpstream(_ string) {
}
