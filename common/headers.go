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

func AddCommonHeadersWithOptions(uc UserConfig, req *http.Request, overwriteAuth bool, includeInternal bool) {

	if includeInternal {
		for k, v := range uc.GetInternalHeaders() {
			if v != "" {
				req.Header.Set(k, v)
			}
		}
	}

	if overwriteAuth || req.Header.Get(DeviceIdHeader) == "" {
		req.Header.Set(DeviceIdHeader, uc.GetDeviceID())
	}
	if overwriteAuth || req.Header.Get(ProTokenHeader) == "" {
		if token := uc.GetToken(); token != "" {
			req.Header.Set(ProTokenHeader, token)
		}
	}
	if overwriteAuth || req.Header.Get(UserIdHeader) == "" {
		req.Header.Set(UserIdHeader, strconv.FormatInt(uc.GetUserID(), 10))
	}

	// unconditionally overwritten
	req.Header.Set(VersionHeader, Version)
}

func AddCommonHeaders(uc UserConfig, req *http.Request) {
	AddCommonHeadersWithOptions(uc, req, true, true)
}
