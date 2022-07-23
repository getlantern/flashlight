package common

import "net/http"

// This lets you turn an request handler into an http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (rt RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}
