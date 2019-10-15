package common

import (
	"net/http"
	"strconv"
)

const (
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

// AddCommonHeaders adds standard http headers the proxy requires.
func AddCommonHeaders(req *http.Request) {
	req.Header.Set(VersionHeader, Version)
	req.Header.Set(PlatformHeader, Platform)
}

// AddAuthHeaders adds the common 'auth' headers to the specific request if
// they are not present.
func AddAuthHeaders(uc UserConfig, req *http.Request) {
	setAuthHeaders(uc, req, false)
}

func setAuthHeaders(uc UserConfig, req *http.Request, overwrite bool) {
	if overwrite || req.Header.Get(DeviceIdHeader) == "" {
		if deviceID := uc.GetDeviceID(); deviceID != "" {
			req.Header.Set(DeviceIdHeader, deviceID)
		}
	}
	if overwrite || req.Header.Get(ProTokenHeader) == "" {
		if token := uc.GetToken(); token != "" {
			req.Header.Set(ProTokenHeader, token)
		}
	}
	if overwrite || req.Header.Get(UserIdHeader) == "" {
		if userID := uc.GetUserID(); userID != 0 {
			req.Header.Set(UserIdHeader, strconv.FormatInt(userID, 10))
		}
	}
}

// addInternalHeaders adds http headers specific to internal services, if any.
func addInternalHeaders(uc UserConfig, req *http.Request) {
	for k, v := range uc.GetInternalHeaders() {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
}

// AddHeadersForInternalServices adds necessary http headers required by
// internal services. overwriteAuth controls whether set auth related headers
// if they are already present.
func AddHeadersForInternalServices(req *http.Request, uc UserConfig, overwriteAuth bool) {
	AddCommonHeaders(req)
	setAuthHeaders(uc, req, overwriteAuth)
	addInternalHeaders(uc, req)
}
