package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getlantern/lantern-server/common"
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

func TestCORS(t *testing.T) {
	uiAddr := "http://localhost:2000"
	cases := []CorsSpec{
		{
			"BadOrigin",
			common.GET,
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
			common.GET,
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
			url := fmt.Sprintf("%s%s", common.AuthStagingAddr, tc.uri)
			req, _ := http.NewRequest(tc.method, url,
				nil)
			for name, value := range tc.reqHeaders {
				req.Header.Add(name, value)
			}
			resp := httptest.NewRecorder()
			CORSMiddleware(testHandler).ServeHTTP(resp, req)
			assertHeaders(t, resp.Header(), tc.respHeaders)
		})
	}
}
