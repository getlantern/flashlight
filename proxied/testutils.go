package proxied

import (
	"net/http"
	"time"
)

type mockRoundTripper_Return200 struct {
	id             FlowComponentID
	processingTime time.Duration
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_Return200) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{
		Header: map[string][]string{
			roundTripperHeaderKey: []string{c.id.String()},
		},
	}
	resp.StatusCode = 200
	return resp, nil
}

type mockRoundTripper_FailOnceAndThenReturn200 struct {
	id             FlowComponentID
	processingTime time.Duration
	failOnce       bool
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_FailOnceAndThenReturn200) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{
		Header: map[string][]string{
			roundTripperHeaderKey: []string{c.id.String()},
		},
	}
	if !c.failOnce {
		resp.StatusCode = 400
		c.failOnce = true
	} else {
		resp.StatusCode = 200
	}
	return resp, nil
}

type mockRoundTripper_Return200Once struct {
	id             FlowComponentID
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
	resp := &http.Response{
		Header: map[string][]string{
			roundTripperHeaderKey: []string{c.id.String()},
		},
	}
	if c.currentCount == c.return200After {
		resp.StatusCode = 200
	} else {
		resp.StatusCode = 400
	}
	c.currentCount++
	return resp, nil
}

type mockRoundTripper_Return400 struct {
	id             FlowComponentID
	processingTime time.Duration
}

// RoundTrip here just sleeps a bit and then returns 200 OK.
// The request is not processed at all
func (c *mockRoundTripper_Return400) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	time.Sleep(c.processingTime)
	resp := &http.Response{
		Header: map[string][]string{
			roundTripperHeaderKey: []string{c.id.String()},
		},
	}
	resp.StatusCode = 400
	return resp, nil
}
