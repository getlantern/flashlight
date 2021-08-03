package chained

import (
	"net/http"

	"github.com/getlantern/eventual"
)

type frontedTransport struct {
	rt eventual.Value
}

func (ft *frontedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt, _ := ft.rt.Get(eventual.Forever)
	return rt.(http.RoundTripper).RoundTrip(req)
}
