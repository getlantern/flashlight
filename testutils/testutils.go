package testutils

import (
	"io"
	"io/ioutil"
	"net/http"
)

type MockRoundTripper struct {
	Req    *http.Request
	Body   io.Reader
	Header http.Header
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.Req = req
	resp := &http.Response{
		StatusCode: 200,
		Header:     m.Header,
		Body:       ioutil.NopCloser(m.Body),
	}
	return resp, nil
}
