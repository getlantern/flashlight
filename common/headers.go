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
