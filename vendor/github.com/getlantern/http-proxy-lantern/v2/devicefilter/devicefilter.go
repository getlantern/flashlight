package devicefilter

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/v2/filters"

	"github.com/getlantern/http-proxy/listeners"

	"github.com/getlantern/http-proxy-lantern/v2/blacklist"
	"github.com/getlantern/http-proxy-lantern/v2/common"
	"github.com/getlantern/http-proxy-lantern/v2/domains"
	"github.com/getlantern/http-proxy-lantern/v2/instrument"
	lanternlisteners "github.com/getlantern/http-proxy-lantern/v2/listeners"
	"github.com/getlantern/http-proxy-lantern/v2/redis"
	"github.com/getlantern/http-proxy-lantern/v2/throttle"
	"github.com/getlantern/http-proxy-lantern/v2/usage"
)

var (
	log = golog.LoggerFor("devicefilter")

	epoch = time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)

	alwaysThrottle = lanternlisteners.NewRateLimiter(10)
)

// deviceFilterPre does the device-based filtering
type deviceFilterPre struct {
	deviceFetcher  *redis.DeviceFetcher
	throttleConfig throttle.Config
	sendXBQHeader  bool
	instrument     instrument.Instrument
}

// deviceFilterPost cleans up
type deviceFilterPost struct {
	bl *blacklist.Blacklist
}

// NewPre creates a filter which throttling all connections from a device if its data usage threshold is reached.
// * df is used to fetch device data usage across all proxies from a central Redis.
// * throttleConfig is to determine the threshold and throttle rate. They can
// be fixed values or fetched from Redis periodically.
// * If sendXBQHeader is true, it attaches a common.XBQHeader to inform the
// clients the usage information before this request is made. The header is
// expected to follow this format:
//
// <used>/<allowed>/<asof>
//
// <used> is the string representation of a 64-bit unsigned integer
// <allowed> is the string representation of a 64-bit unsigned integer
// <asof> is the 64-bit signed integer representing seconds since a custom
// epoch (00:00:00 01/01/2016 UTC).

func NewPre(df *redis.DeviceFetcher, throttleConfig throttle.Config, sendXBQHeader bool, instrument instrument.Instrument) filters.Filter {
	if throttleConfig != nil {
		log.Debug("Throttling enabled")
	}

	return &deviceFilterPre{
		deviceFetcher:  df,
		throttleConfig: throttleConfig,
		sendXBQHeader:  sendXBQHeader,
		instrument:     instrument,
	}
}

func (f *deviceFilterPre) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("DeviceFilter Middleware received request:\n%s", reqStr)
	}

	// Some domains are excluded from being throttled and don't count towards the
	// bandwidth cap.
	if domains.ConfigForRequest(req).Unthrottled {
		f.instrument.Throttle(false, "domain-excluded")
		return next(cs, req)
	}

	// Attached the uid to connection to report stats to redis correctly
	// "conn" in context is previously attached in server.go
	wc := cs.Downstream().(listeners.WrapConn)
	lanternDeviceID := req.Header.Get(common.DeviceIdHeader)

	if lanternDeviceID == "" {
		// Old lantern versions and possible cracks do not include the device
		// ID. Just throttle them.
		f.instrument.Throttle(true, "no-device-id")
		wc.ControlMessage("throttle", alwaysThrottle)
		return next(cs, req)
	}
	if lanternDeviceID == "~~~~~~" {
		// This is checkfallbacks, don't throttle it
		f.instrument.Throttle(false, "checkfallbacks")
		return next(cs, req)
	}

	if f.throttleConfig == nil {
		f.instrument.Throttle(false, "no-config")
		return next(cs, req)
	}

	// Throttling enabled
	u := usage.Get(lanternDeviceID)
	if u == nil {
		// Eagerly request device ID data from Redis and store it in usage
		f.deviceFetcher.RequestNewDeviceUsage(lanternDeviceID)
		f.instrument.Throttle(false, "no-usage-data")
		return next(cs, req)
	}

	settings, capOn := f.throttleConfig.SettingsFor(lanternDeviceID, u.CountryCode, req.Header.Get(common.PlatformHeader), req.Header.Get(common.AppHeader), req.Header[common.SupportedDataCaps])

	measuredCtx := map[string]interface{}{
		"throttled": false,
	}

	// To turn the data cap off in Redis we simply set the threshold to 0 or
	// below. This will also turn off the cap in the UI on desktop and in newer
	// versions on mobile.
	if capOn {
		log.Tracef("Got throttle settings: %v", settings)
		capOn = settings.Threshold > 0

		// Send throttle settings to measured as well
		measuredCtx["throttle_settings"] = settings
	}

	if capOn && u.Bytes > settings.Threshold {
		// per connection limiter
		limiter := lanternlisteners.NewRateLimiter(settings.Rate)
		log.Tracef("Throttling connection from device %s to %v per second", lanternDeviceID,
			humanize.Bytes(uint64(settings.Rate)))
		f.instrument.Throttle(true, "datacap")
		wc.ControlMessage("throttle", limiter)
		measuredCtx["throttled"] = true
	} else {
		// default case is not throttling
		f.instrument.Throttle(false, "")
	}
	wc.ControlMessage("measured", measuredCtx)

	resp, nextCtx, err := next(cs, req)
	if resp == nil || err != nil {
		return resp, nextCtx, err
	}
	if !capOn || !f.sendXBQHeader {
		return resp, nextCtx, err
	}
	if resp.Header == nil {
		resp.Header = make(http.Header, 1)
	}
	uMiB := u.Bytes / (1024 * 1024)
	xbq := fmt.Sprintf("%d/%d/%d", uMiB, settings.Threshold/(1024*1024), int64(u.AsOf.Sub(epoch).Seconds()))
	xbqv2 := fmt.Sprintf("%s/%d", xbq, u.TTLSeconds)
	resp.Header.Set(common.XBQHeader, xbq)     // for backward compatibility with older clients
	resp.Header.Set(common.XBQHeaderv2, xbqv2) // for new clients that support different bandwidth cap expirations
	f.instrument.XBQHeaderSent()
	return resp, nextCtx, err
}

func NewPost(bl *blacklist.Blacklist) filters.Filter {
	return &deviceFilterPost{
		bl: bl,
	}
}

func (f *deviceFilterPost) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	// For privacy, delete the DeviceId header before passing it along
	req.Header.Del(common.DeviceIdHeader)
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	f.bl.Succeed(ip)
	return next(cs, req)
}
