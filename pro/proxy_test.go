package pro

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
)

func TestProxy(t *testing.T) {
	uc := common.NewUserConfigData("device", 0, "token", nil, "en-US")
	m := &mockRoundTripper{header: http.Header{}, body: strings.NewReader("GOOD")}
	httpClient = &http.Client{Transport: m}
	l, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}

	addr := l.Addr()
	url := fmt.Sprintf("http://%s/pro/abc", addr)
	t.Logf("Test server listening at %s", url)
	go http.Serve(l, APIHandler(uc))

	req, err := http.NewRequest("OPTIONS", url, nil)
	if !assert.NoError(t, err) {
		return
	}

	req.Header.Set("Origin", "a.com")
	resp, err := (&http.Client{}).Do(req)
	if assert.NoError(t, err, "OPTIONS request should succeed") {
		assert.Equal(t, 200, resp.StatusCode, "should respond 200 to OPTIONS")
		assert.Equal(t, "a.com", resp.Header.Get("Access-Control-Allow-Origin"), "should respond with correct header")
		_ = resp.Body.Close()
	}
	assert.Nil(t, m.req, "should not pass the OPTIONS request to origin server")

	req, err = http.NewRequest("GET", url, nil)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Set("Origin", "a.com")
	resp, err = (&http.Client{}).Do(req)
	if assert.NoError(t, err, "GET request should have no error") {
		assert.Equal(t, 200, resp.StatusCode, "should respond 200 ok")
		assert.Equal(t, "a.com", resp.Header.Get("Access-Control-Allow-Origin"), "should respond with correct header")
		msg, _ := ioutil.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, "GOOD", string(msg), "should respond expected body")
	}
	if assert.NotNil(t, m.req, "should pass through non-OPTIONS requests to origin server") {
		t.Log(m.req)
		assert.Empty(t, m.req.Header.Get("Origin"), "should strip off Origin header")
	}

	url = fmt.Sprintf("http://%s/pro/user-data", addr)
	msg, _ := json.Marshal(&client.User{Email: "a@a.com"})
	m.body = bytes.NewReader(msg)
	req, err = http.NewRequest("GET", url, nil)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Set("X-Lantern-User-Id", "1234")
	resp, err = (&http.Client{}).Do(req)
	if assert.NoError(t, err, "GET request should have no error") {
		assert.Equal(t, 200, resp.StatusCode, "should respond 200 ok")
	}
	user, found := GetUserDataFast(1234)
	if assert.True(t, found) {
		assert.Equal(t, "a@a.com", user.Email, "should store user data implicitly if response is plain JSON")
	}

	var gzipped bytes.Buffer
	gw := gzip.NewWriter(&gzipped)
	msg, _ = json.Marshal(&client.User{Email: "b@b.com"})
	io.Copy(gw, bytes.NewReader(msg))
	gw.Close()
	m.body = &gzipped
	m.header.Set("Content-Encoding", "gzip")
	resp, err = (&http.Client{}).Do(req)
	if assert.NoError(t, err, "GET request should have no error") {
		assert.Equal(t, 200, resp.StatusCode, "should respond 200 ok")
	}
	user, found = GetUserDataFast(1234)
	if assert.True(t, found) {
		assert.Equal(t, "b@b.com", user.Email, "should store user data implicitly if response is gzipped JSON")
	}

}

type mockRoundTripper struct {
	req    *http.Request
	body   io.Reader
	header http.Header
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.req = req
	resp := &http.Response{
		StatusCode: 200,
		Header:     m.header,
		Body:       ioutil.NopCloser(m.body),
	}
	return resp, nil
}
