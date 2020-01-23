package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/common"
)

type CorsSpec struct {
	name        string
	method      string
	uri         string
	reqHeaders  map[string]string
	respHeaders map[string]string
}

var allHeaders = []string{
	"Vary",
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Credentials",
	"Access-Control-Max-Age",
	"Access-Control-Expose-Headers",
}

var testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("test"))
})

func assertHeaders(t *testing.T, resHeaders http.Header, expHeaders map[string]string) {
	for _, name := range allHeaders {
		got := strings.Join(resHeaders[name], ", ")
		want := expHeaders[name]
		if got != want {
			t.Errorf("Response header %q = %q, want %q", name, got, want)
		}
	}
}

func dumpResponse(resp *httptest.ResponseRecorder) {
	result := resp.Result()
	dump, _ := httputil.DumpResponse(result, true)
	log.Debugf("HTTP response is %q", dump)
}

func TestCors(t *testing.T) {
	s := startServer(t, common.AuthServerAddr, ":0")
	uiAddr := fmt.Sprintf("http://%s", s.GetUIAddr())
	cases := []CorsSpec{
		{
			"BadOrigin",
			GET,
			"/login",
			map[string]string{
				"Origin": "http://baddomain.com",
			},
			map[string]string{
				"Vary":                        "Origin",
				"Access-Control-Allow-Origin": "",
			},
		},
		{
			"AllowedOrigin",
			GET,
			"/login",
			map[string]string{
				"Origin": uiAddr,
			},
			map[string]string{
				"Vary":                             "Origin",
				"Access-Control-Allow-Credentials": "true",
				"Access-Control-Allow-Origin":      uiAddr,
			},
		},
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			url := s.getAPIAddr(tc.uri)
			req, _ := http.NewRequest(tc.method, url,
				nil)
			for name, value := range tc.reqHeaders {
				req.Header.Add(name, value)
			}
			resp := httptest.NewRecorder()
			s.corsHandler(testHandler).ServeHTTP(resp, req)
			dumpResponse(resp)
			assertHeaders(t, resp.Header(), tc.respHeaders)
		})
	}
}
