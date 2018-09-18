package pro

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/zaplog"
)

var (
	log = zaplog.LoggerFor("flashlight.pro")
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
		resp, err = GetHTTPClient().Do(req)
		if err != nil {
			log.Errorf("Could not issue HTTP request? %v", err)
			return
		}
	}
	resp.Header.Set("Access-Control-Allow-Origin", origin)
	if req.URL.Path != "/user-data" || resp.StatusCode != http.StatusOK {
		return
	}
	// Try to update user data implicitly
	_userID := req.Header.Get("X-Lantern-User-Id")
	if _userID == "" {
		log.Error("Request has an empty user ID")
		return
	}
	userID, parseErr := strconv.ParseInt(_userID, 10, 64)
	if parseErr != nil {
		log.Errorf("User ID %s is invalid", _userID)
		return
	}
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error read response body: %v", readErr)
		return
	}
	resp.Body = ioutil.NopCloser(bytes.NewReader(body))
	encoding := resp.Header.Get("Content-Encoding")
	var br io.Reader = bytes.NewReader(body)
	switch encoding {
	case "gzip":
		gzr, readErr := gzip.NewReader(bytes.NewReader(body))
		if readErr != nil {
			log.Errorf("Unable to decode gzipped data: %v", readErr)
			return
		}
		br = gzr
	case "":
	default:
		log.Errorf("Unsupported response encoding %s", encoding)
		return
	}
	user := client.User{}
	readErr = json.NewDecoder(br).Decode(&user)
	if readErr != nil {
		log.Errorf("Error decode JSON: %v", readErr)
		return
	}
	log.Infof("Updating user data implicitly for user %v", userID)
	setUserData(userID, &user)
	return
}

func PrepareProRequest(r *http.Request, uc common.UserConfig) {
	r.URL.Scheme = "https"
	r.URL.Host = common.ProAPIHost
	r.Host = r.URL.Host
	r.RequestURI = "" // http: Request.RequestURI can't be set in client requests.
	r.Header.Set("Access-Control-Allow-Headers", strings.Join([]string{
		common.DeviceIdHeader,
		common.ProTokenHeader,
		common.UserIdHeader,
	}, ", "))

	// Add auth headers only if not present, to avoid race conditions
	// when creating new user or switching user, i.e., linking device
	// to a new account. (ovewriteAuth=false)
	common.AddCommonHeadersWithOptions(uc, r, false)
}

// APIHandler returns an HTTP handler that specifically looks for and properly
// handles pro server requests.
func APIHandler(uc common.UserConfig) http.Handler {
	log.Infof("Returning pro API handler hitting host: %v", common.ProAPIHost)
	return &httputil.ReverseProxy{
		Transport: &proxyTransport{},
		Director: func(r *http.Request) {
			// Strip /pro from path.
			if strings.HasPrefix(r.URL.Path, "/pro/") {
				r.URL.Path = r.URL.Path[4:]
			}
			PrepareProRequest(r, uc)
		},
	}
}
