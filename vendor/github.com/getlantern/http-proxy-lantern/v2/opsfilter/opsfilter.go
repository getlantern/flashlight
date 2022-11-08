package opsfilter

import (
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxy/v2"
	"github.com/getlantern/proxy/v2/filters"

	"github.com/getlantern/http-proxy-lantern/v2/bbr"
	"github.com/getlantern/http-proxy-lantern/v2/common"
	"github.com/getlantern/http-proxy/listeners"
)

var (
	log = golog.LoggerFor("logging")
)

type opsfilter struct {
	bm bbr.Middleware
}

// New constructs a new filter that adds ops context.
func New(bm bbr.Middleware) filters.Filter {
	return &opsfilter{bm}
}

func (f *opsfilter) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	deviceID := req.Header.Get(common.DeviceIdHeader)
	originHost, originPort, _ := net.SplitHostPort(req.Host)
	if (originPort == "0" || originPort == "") && req.Method != http.MethodConnect {
		// Default port for HTTP
		originPort = "80"
	}
	if originHost == "" && !strings.Contains(req.Host, ":") {
		originHost = req.Host
	}
	platform := req.Header.Get(common.PlatformHeader)
	version := req.Header.Get(common.VersionHeader)
	app := req.Header.Get(common.AppHeader)

	op := ops.Begin("proxy").
		Set("device_id", deviceID).
		Set("origin", req.Host).
		Set("origin_host", originHost).
		Set("origin_port", originPort).
		Set("proxy_dial_timeout", req.Header.Get(proxy.DialTimeoutHeader)).
		Set("app_platform", platform).
		Set("app_version", version).
		Set("client_app", app)
	log.Tracef("Starting op")
	defer op.End()

	measuredCtx := map[string]interface{}{
		"origin":      req.Host,
		"origin_host": originHost,
		"origin_port": originPort,
	}

	addMeasuredHeader := func(key string, headerValue interface{}) {
		if headerValue != nil && headerValue != "" {
			headerArray, ok := headerValue.([]string)
			if ok && len(headerArray) == 0 {
				return
			}
			measuredCtx[key] = headerValue
		}
	}

	// On persistent HTTP connections, some or all of the below may be missing on requests after the first. By only setting
	// the values when they're available, the measured listener will preserve any values that were already included in the
	// first request on the connection.
	addMeasuredHeader("deviceid", deviceID)
	addMeasuredHeader("app_version", version)
	addMeasuredHeader("app_platform", platform)
	addMeasuredHeader("app", app)
	addMeasuredHeader("supported_data_caps", req.Header[common.SupportedDataCaps])
	addMeasuredHeader("time_zone", req.Header.Get(common.TimeZoneHeader))

	clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		clientIP = req.RemoteAddr
	}
	op.Set("client_ip", clientIP)
	measuredCtx["client_ip"] = clientIP

	// Send the same context data to measured as well
	wc := cs.Downstream().(listeners.WrapConn)
	wc.ControlMessage("measured", measuredCtx)

	resp, nextCtx, nextErr := next(cs, req)
	op.FailIf(nextErr)

	return resp, nextCtx, nextErr
}
