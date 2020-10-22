package common

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	AppHeader                           = "X-Lantern-App"
	VersionHeader                       = "X-Lantern-Version"
	DeviceIdHeader                      = "X-Lantern-Device-Id"
	TokenHeader                         = "X-Lantern-Auth-Token"
	UserIdHeader                        = "X-Lantern-User-Id"
	ProTokenHeader                      = "X-Lantern-Pro-Token"
	CfgSvrAuthTokenHeader               = "X-Lantern-Config-Auth-Token"
	CfgSvrClientIPHeader                = "X-Lantern-Config-Client-IP"
	BBRBytesSentHeader                  = "X-BBR-Sent"
	BBRAvailableBandwidthEstimateHeader = "X-BBR-ABE"
	EtagHeader                          = "X-Lantern-Etag"
	IfNoneMatchHeader                   = "X-Lantern-If-None-Match"
	PingHeader                          = "X-Lantern-Ping"
	PlatformHeader                      = "X-Lantern-Platform"
	ProxyDialTimeoutHeader              = "X-Lantern-Dial-Timeout"
	ClientCountryHeader                 = "X-Lantern-Client-Country"
)

// AddCommonHeadersWithOptions sets standard http headers on a request bound
// for an internal service, representing auth and other configuration
// metadata.  The caller may specify overwriteAuth=false to prevent overwriting
// any of the common 'auth' headers (DeviceIdHeader, ProTokenHeader, UserIdHeader)
// that are already present in the given request.
func AddCommonHeadersWithOptions(uc UserConfig, req *http.Request, overwriteAuth bool) {
	req.Header.Set(VersionHeader, Version)
	for k, v := range uc.GetInternalHeaders() {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	req.Header.Set(PlatformHeader, Platform)
	req.Header.Set(AppHeader, AppName)

	if overwriteAuth || req.Header.Get(DeviceIdHeader) == "" {
		if deviceID := uc.GetDeviceID(); deviceID != "" {
			req.Header.Set(DeviceIdHeader, deviceID)
		}
	}
	if overwriteAuth || req.Header.Get(ProTokenHeader) == "" {
		if token := uc.GetToken(); token != "" {
			req.Header.Set(ProTokenHeader, token)
		}
	}
	if overwriteAuth || req.Header.Get(UserIdHeader) == "" {
		if userID := uc.GetUserID(); userID != 0 {
			req.Header.Set(UserIdHeader, strconv.FormatInt(userID, 10))
		}
	}
}

// AddCommonHeaders sets standard http headers on a request
// bound for an internal service, representing auth and other
// configuration metadata.
func AddCommonHeaders(uc UserConfig, req *http.Request) {
	AddCommonHeadersWithOptions(uc, req, true)
}

// ProcessCORS processes CORS requests on localhost.
func ProcessCORS(responseHeaders http.Header, r *http.Request) {
	origin := r.Header.Get("origin")
	if origin == "" {
		log.Debugf("Request is not a CORS request")
		return
	}
	// The origin can have include arbitrary ports, so we just make sure
	// it's on localhost.
	if strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "http://[::1]:") {

		responseHeaders.Set("Access-Control-Allow-Origin", origin)
		responseHeaders.Add("Access-Control-Allow-Methods", "GET")
		responseHeaders.Add("Access-Control-Allow-Methods", "POST")
		responseHeaders.Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
	}
}
