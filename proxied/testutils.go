package proxied

import (
	"net/http"
	"time"
)

const (
	roundTripperHeaderKey = "X-Lantern-RoundTripper"
)

// OnStartRoundTrip is called by the flow when it starts a new roundtrip.
type OnStartRoundTripFunc func(FlowComponentID, *http.Request)

// OnCompleteRoundTrip is called by the flow when it completes a roundtrip.
type OnCompleteRoundTripFunc func(FlowComponentID)

type mockRoundTripper_Return200 struct {
	id             FlowComponentID
	processingTime time.Duration
}

type withTestInfo struct {
	rt                  http.RoundTripper
	onStartRoundTrip    OnStartRoundTripFunc
	onCompleteRoundTrip OnCompleteRoundTripFunc
	id                  FlowComponentID
}

func (r *withTestInfo) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.onStartRoundTrip != nil {
		r.onStartRoundTrip(r.id, req)
	}
	if r.onCompleteRoundTrip != nil {
		defer r.onCompleteRoundTrip(r.id)
	}

	resp, err := r.rt.RoundTrip(req)
	if resp != nil {
		if resp.Header == nil {
			resp.Header = http.Header{}
		}
		resp.Header.Add(roundTripperHeaderKey, string(r.id))
	}

	return resp, err
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_Return200) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{
		StatusCode: 200,
	}
	return resp, nil
}

type mockRoundTripper_FailOnceAndThenReturn200 struct {
	processingTime time.Duration
	failOnce       bool
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_FailOnceAndThenReturn200) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{}
	if !c.failOnce {
		resp.StatusCode = 400
		c.failOnce = true
	} else {
		resp.StatusCode = 200
	}
	return resp, nil
}

type mockRoundTripper_Return200Once struct {
	processingTime time.Duration
	return200After int
	currentCount   int
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_Return200Once) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{}
	if c.currentCount == c.return200After {
		resp.StatusCode = 200
	} else {
		resp.StatusCode = 400
	}
	c.currentCount++
	return resp, nil
}

type mockRoundTripper_Return400 struct {
	processingTime time.Duration
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_Return400) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{}
	resp.StatusCode = 400
	return resp, nil
}
