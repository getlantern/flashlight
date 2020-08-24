package desktopReplica

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/internal/testutils"
	"github.com/stretchr/testify/assert"
)

func TestSearchProxy(t *testing.T) {
	uc := common.NewUserConfigData("device", 0, "token", nil, "en-US")

	m := &testutils.MockRoundTripper{Header: http.Header{}, Body: strings.NewReader("GOOD")}
	httpClient = &http.Client{Transport: m}

	l, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}
	addr := l.Addr()
	url := fmt.Sprintf("http://%s/replica/search", addr)
	t.Logf("Test server listening at %s", url)

	handler := searchHandler(uc)
	go http.Serve(l, handler)

	{
		req, err := http.NewRequest("OPTIONS", url, nil)
		if !assert.NoError(t, err) {
			return
		}
		req.Header.Set("Origin", "a.com")
		resp, err := (&http.Client{}).Do(req)
		if assert.NoError(t, err, "OPTIONS request should succeed") {
			assert.Equal(t, 200, resp.StatusCode, "should respond 200 to OPTIONS")
			assert.Equal(t, "GET", resp.Header.Get("Access-Control-Allow-Methods"), "should respond with correct CORS method header")
			_ = resp.Body.Close()
		}
		assert.Nil(t, m.Req, "should not pass the OPTIONS request to origin server")
	}

	{
		req, err := http.NewRequest("GET", url, nil)
		if !assert.NoError(t, err) {
			return
		}
		req.Header.Set("Origin", "a.com")
		resp, err := (&http.Client{}).Do(req)
		if assert.NoError(t, err, "GET request should succeed") {
			assert.Equal(t, 200, resp.StatusCode, "should respond 200 to GET")
			_ = resp.Body.Close()
		}
		if assert.NotNil(t, m.Req, "should pass through non-OPTIONS requests to origin server") {
			t.Log(m.Req)
			assert.Equal(t, "device", m.Req.Header.Get("x-lantern-device-id"), "should include device id header in request to search api")
			assert.Equal(t, "token", m.Req.Header.Get("x-lantern-pro-token"), "should include pro token header in request to search api")
		}
	}
}
