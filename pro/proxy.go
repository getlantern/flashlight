package pro

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/pro/client"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.pro")
)

type proxyTransport struct {
	httpClient *http.Client
}

func (pt *proxyTransport) processOptions(req *http.Request) *http.Response {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Connection": {"keep-alive"},
			"Via":        {"Lantern Client"},
		},
		Body: io.NopCloser(strings.NewReader("preflight complete")),
	}
	if !common.ProcessCORS(resp.Header, req) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
		}
	}
	return resp
}

func (pt *proxyTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if req.Method == "OPTIONS" {
		// No need to proxy the OPTIONS request.
		return pt.processOptions(req), nil
	}
	origin := req.Header.Get("Origin")
	// Workaround for https://github.com/getlantern/pro-server/issues/192
	req.Header.Del("Origin")
	resp, err = pt.httpClient.Do(req)
	if err != nil {
		log.Errorf("Could not issue HTTP request? %v", err)
		return
	}

	// Put the header back for subsequent CORS processing.
	req.Header.Set("Origin", origin)
	common.ProcessCORS(resp.Header, req)
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
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error read response body: %v", readErr)
		return
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
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
		log.Errorf("Error decoding JSON: %v", readErr)
		return
	}
	log.Debugf("Updating user data implicitly for user %v", userID)
	setUserData(userID, &user)
	return
}

// PrepareProRequestWithOptions normalizes requests to the pro server with
// device ID, user ID, etc set.
func PrepareProRequestWithOptions(r *http.Request, uc common.UserConfig) {
	prepareProRequest(r, uc, true)
}

// PrepareProRequest normalizes requests to the pro server without overwriting
// device ID, user ID, etc.
func PrepareProRequest(r *http.Request, uc common.UserConfig) {
	prepareProRequest(r, uc, false)
}

func prepareProRequest(r *http.Request, uc common.UserConfig, options bool) {
	r.URL.Scheme = "http"
	r.URL.Host = common.ProAPIHost
	// XXX <03-02-22, soltzen> Requests coming from lantern-desktop's UI client
	// will always carry lantern-desktop's server address (i.e.,
	// [here](https://github.com/getlantern/lantern-desktop/blob/87370cca9c895d0e0296b4d16e292ad8adbdae33/server/defaults_static.go#L1))
	// in their 'Host' header (like this: 'Host: localhost:16823'). This is
	// problamatic for many servers (Replica's as well). So, best to either
	// wipe it or assign it as the URL's host
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
	common.AddCommonHeadersWithOptions(uc, r, options)
}

// APIHandler returns an HTTP handler that specifically looks for and properly
// handles pro server requests.
func APIHandlerWithClient(uc common.UserConfig, httpClient *http.Client) http.Handler {
	log.Debugf("Returning pro API handler hitting host: %v", common.ProAPIHost)
	return &httputil.ReverseProxy{
		Transport: &proxyTransport{
			httpClient: httpClient,
		},
		Director: func(r *http.Request) {
			// Strip /pro from path.
			if strings.HasPrefix(r.URL.Path, "/pro/") {
				r.URL.Path = r.URL.Path[4:]
			}
			PrepareProRequest(r, uc)
		},
	}
}

// APIHandler returns an HTTP handler that specifically looks for and properly
// handles pro server requests.
func APIHandler(uc common.UserConfig) http.Handler {
	return APIHandlerWithClient(uc, common.GetHTTPClient())
}
