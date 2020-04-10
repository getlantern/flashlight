package testutils

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

var (
	log = golog.LoggerFor("flashlight.ui.testutils")
)

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

func DumpRequestHeaders(r *http.Request) {
	dump, err := httputil.DumpRequest(r, false)
	if err == nil {
		log.Debugf("Request:\n%s", string(dump))
	}
}
