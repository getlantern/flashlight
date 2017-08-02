package pro

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
)

var (
	log        = golog.LoggerFor("flashlight.pro")
	httpClient = &http.Client{Transport: proxied.ChainedThenFronted()}
	// Respond sooner if chained proxy is blocked, but only for idempotent requests (GETs)
	httpClientForGET = &http.Client{Transport: proxied.ParallelPreferChained()}
)

type proxyTransport struct {
	// Satisfies http.RoundTripper
}

func (pt *proxyTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	origin := req.Header.Get("Origin")
	if req.Method == "OPTIONS" {
		// No need to proxy the OPTIONS request.
		resp = &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Connection":                   {"keep-alive"},
				"Access-Control-Allow-Methods": {"GET, POST"},
				"Access-Control-Allow-Headers": {req.Header.Get("Access-Control-Request-Headers")},
				"Via": {"Lantern Client"},
			},
			Body: ioutil.NopCloser(strings.NewReader("preflight complete")),
		}
	} else {
		// Workaround for https://github.com/getlantern/pro-server/issues/192
		req.Header.Del("Origin")
		if req.Method == "GET" {
			resp, err = httpClientForGET.Do(req)
		} else {
			resp, err = httpClient.Do(req)
		}
		if err != nil {
			log.Errorf("Could not issue HTTP request? %v", err)
			return
		}
	}
	resp.Header.Set("Access-Control-Allow-Origin", origin)
	if req.URL.Path != "/user-data" || resp.StatusCode != http.StatusOK {
		return
	}
	// Try to update user status implicitly
	_userID := req.Header.Get("X-Lantern-User-Id")
	if _userID == "" {
		return
	}
	userID, parseErr := strconv.ParseInt(_userID, 10, 64)
	if parseErr != nil {
		return
	}
	gzbody, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		return
	}
	resp.Body = ioutil.NopCloser(bytes.NewReader(gzbody))
	gzr, readErr := gzip.NewReader(bytes.NewReader(gzbody))
	if readErr != nil {
		return
	}
	user := client.User{}
	readErr = json.NewDecoder(gzr).Decode(&user)
	if readErr != nil {
		return
	}
	log.Debugf("Updating user data implicitly for user %v", userID)
	setUserData(userID, user)
	return
}

// APIHandler returns an HTTP handler that specifically looks for and properly
// handles pro server requests.
func APIHandler() http.Handler {
	log.Debugf("Returning pro API handler hitting host: %v", common.ProAPIHost)
	return &httputil.ReverseProxy{
		Transport: &proxyTransport{},
		Director: func(r *http.Request) {
			// Strip /pro from path.
			if strings.HasPrefix(r.URL.Path, "/pro/") {
				r.URL.Path = r.URL.Path[4:]
			}
			r.URL.Scheme = "https"
			r.URL.Host = common.ProAPIHost
			r.Host = r.URL.Host
			r.RequestURI = "" // http: Request.RequestURI can't be set in client requests.
			r.Header.Set("Lantern-Fronted-URL", fmt.Sprintf("http://%s%s", common.ProAPIDDFHost, r.URL.Path))
			r.Header.Set("Access-Control-Allow-Headers", strings.Join([]string{
				common.DeviceIdHeader,
				common.ProTokenHeader,
				common.UserIdHeader,
			}, ", "))
		},
	}
}
