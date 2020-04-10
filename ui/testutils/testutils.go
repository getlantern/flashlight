package testutils

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

var (
	log = golog.LoggerFor("flashlight.ui.testutils")
)

// StartTestServer starts a new test UI server on the given addr
func StartTestServer(t *testing.T, authaddr, addr string) *ui.Server {
	s := ui.NewServer(ui.ServerParams{
		AuthServerAddr: authaddr,
		LocalHTTPToken: "test-http-token",
		HTTPClient:     http.DefaultClient,
	})
	assert.NoError(t, s.Start(addr), "should start server")
	return s
}

// DecodeResp is used to decode a httptest Response to the given interface r
func DecodeResp(t *testing.T, resp *httptest.ResponseRecorder, r interface{}) {
	body, _ := ioutil.ReadAll(resp.Body)
	err := json.Unmarshal(body, r)
	assert.Nil(t, err)
}

// DumpResponse dumps the test HTTP response resp
func DumpResponse(resp *httptest.ResponseRecorder) {
	result := resp.Result()
	dump, _ := httputil.DumpResponse(result, true)
	log.Debugf("HTTP response is %q", dump)
}
