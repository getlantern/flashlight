// Package android implements the mobile application functionality of flashlight
package android

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/autoupdate"
	"github.com/getlantern/bandwidth"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/golog"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/protected"
)

const (
	maxDNSGrabCache = 10000
)

var (
	log = golog.LoggerFor("lantern")

	updateServerURL = "https://update.getlantern.org"
	defaultLocale   = `en-US`

	startOnce sync.Once

	cl   *client.Client
	clMu sync.Mutex
)

type Settings interface {
	StickyConfig() bool
	EnableAdBlocking() bool
	DefaultDnsServer() string
	GetHttpProxyHost() string
	GetHttpProxyPort() int
	TimeoutMillis() int
}

type Session interface {
	common.AuthConfig
	SetCountry(string)
	UpdateAdSettings(AdSettings)
	UpdateStats(string, string, string, int, int)
	SetStaging(bool)
	ProxyAll() bool
	BandwidthUpdate(int, int, int)
	Locale() string
	Code() string
	GetCountryCode() string
	GetDNSServer() string
	Provider() string
	AppVersion() string
	IsPlayVersion() bool
	Email() string
	SetCode(string)
	Currency() string
	DeviceOS() string
	IsProUser() bool

	// workaround for lack of any sequence types in gomobile bind... ;_;
	// used to implement GetInternalHeaders() map[string]string
	// Should return a JSON encoded map[string]string {"key":"val","key2":"val", ...}
	SerializedInternalHeaders() string
}

type userConfig struct {
	session Session
}

func (uc *userConfig) GetDeviceID() string { return uc.session.GetDeviceID() }
func (uc *userConfig) GetUserID() int64    { return uc.session.GetUserID() }
func (uc *userConfig) GetToken() string    { return uc.session.GetToken() }
func (uc *userConfig) GetInternalHeaders() map[string]string {
	h := make(map[string]string)

	var f interface{}
	if err := json.Unmarshal([]byte(uc.session.SerializedInternalHeaders()), &f); err != nil {
		return h
	}
	m, ok := f.(map[string]interface{})
	if !ok {
		return h
	}

	for k, v := range m {
		vv, ok := v.(string)
		if ok {
			h[k] = vv
		}
	}
	return h
}

func newUserConfig(session Session) *userConfig {
	return &userConfig{session: session}
}

// SocketProtector is an interface for classes that can protect Android sockets,
// meaning those sockets will not be passed through the VPN.
type SocketProtector interface {
	ProtectConn(fileDescriptor int) error
}

// ProtectConnections allows connections made by Lantern to be protected from
// routing via a VPN. This is useful when running Lantern as a VPN on Android,
// because it keeps Lantern's own connections from being captured by the VPN and
// resulting in an infinite loop.

// The DNS server is used to resolve host only when dialing a protected connection
// from within Lantern client.
func ProtectConnections(protector SocketProtector, dnsServer string) {
	p := protected.New(protector.ProtectConn, dnsServer)
	netx.OverrideDial(p.DialContext)
	netx.OverrideDialUDP(p.DialUDP)
	netx.OverrideResolve(p.ResolveTCP)
	netx.OverrideResolveUDP(p.ResolveUDP)
	clMu.Lock()
	defer clMu.Unlock()
	if cl != nil && cl.GetBalancer() != nil {
		cl.GetBalancer().ForceRedial()
	}
}

// RemoveOverrides removes the protected tlsdialer overrides
// that allowed connections to bypass the VPN.
func RemoveOverrides() {
	netx.Reset()
}

type SurveyInfo struct {
	Enabled     bool    `json:"enabled"`
	Probability float64 `json:"probability"`
	Campaign    string  `json:"campaign"`
	Url         string  `json:"url"`
	Message     string  `json:"message"`
	Thanks      string  `json:"thanks"`
	Button      string  `json:"button"`
}

// StartResult provides information about the started Lantern
type StartResult struct {
	HTTPAddr   string
	SOCKS5Addr string
}

// AdSettings is an interface for retrieving mobile ad settings from the
// global config
type AdSettings interface {
	AppId() string
	AdunitId() string
	VideoAdunitId() string
	InterstitialAdId() string
	Enabled() bool
	GetPercentage() float64
	GetProvider() string
	NativeAdId() string
	GetMinDaysShowAds() int
	GetMaxDaysShowAds() int
	IsRegionEnabled(string) bool
	IsWhitelisted(string) bool
	UseWhitelist() bool
}

type Updater autoupdate.Updater

// Start starts a HTTP and SOCKS proxies at random addresses. It blocks up till
// the given timeout waiting for the proxy to listen, and returns the addresses
// at which it is listening (HTTP, SOCKS). If the proxy doesn't start within the
// given timeout, this method returns an error.
//
// If a Lantern proxy is already running within this process, that proxy is
// reused.
//
// Note - this does not wait for the entire initialization sequence to finish,
// just for the proxy to be listening. Once the proxy is listening, one can
// start to use it, even as it finishes its initialization sequence. However,
// initial activity may be slow, so clients with low read timeouts may
// time out.
func Start(configDir string, locale string,
	settings Settings, session Session) (*StartResult, error) {

	startOnce.Do(func() {
		go run(configDir, locale, settings, session)
	})

	elapsed := mtime.Stopwatch()
	addr, ok := client.Addr(time.Duration(settings.TimeoutMillis()) * time.Millisecond)
	if !ok {
		return nil, fmt.Errorf("HTTP Proxy didn't start within %vms timeout", settings.TimeoutMillis())
	}

	socksAddr, ok := client.Socks5Addr((time.Duration(settings.TimeoutMillis()) * time.Millisecond) - elapsed())
	if !ok {
		err := fmt.Errorf("SOCKS5 Proxy didn't start within %vms timeout", settings.TimeoutMillis())
		log.Error(err.Error())
		return nil, err
	}
	log.Debugf("Starting socks proxy at %s", socksAddr)

	return &StartResult{addr.(string), socksAddr.(string)}, nil
}

// AddLoggingMetadata adds metadata for reporting to cloud logging services
func AddLoggingMetadata(key, value string) {
	//logging.SetExtraLogglyInfo(key, value)
}

// Exit is used to immediately stop Lantern; it's called when the
// the service Lantern is running in receives a signal to
// stop (such as when a user s on another VPN)
func Exit() {
	os.Exit(0)
}

func run(configDir, locale string,
	settings Settings, session Session) {

	appdir.SetHomeDir(configDir)
	session.SetStaging(common.Staging)

	log.Debugf("Starting lantern: configDir %s locale %s sticky config %t",
		configDir, locale, settings.StickyConfig())

	flags := map[string]interface{}{
		"borda-report-interval":    5 * time.Minute,
		"borda-sample-percentage":  float64(0.01),
		"loggly-sample-percentage": float64(0.02),
		"staging":                  common.Staging,
	}

	err := os.MkdirAll(configDir, 0755)
	if os.IsExist(err) {
		log.Errorf("Unable to create configDir at %v: %v", configDir, err)
		return
	}

	if settings.StickyConfig() {
		flags["stickyconfig"] = true
		flags["readableconfig"] = true
	}

	logging.EnableFileLogging(configDir)

	log.Debugf("Writing log messages to %s/lantern.log", configDir)

	grabber, err := dnsgrab.Listen(maxDNSGrabCache,
		":8153",
		session.GetDNSServer())
	if err != nil {
		log.Errorf("Unable to start dnsgrab: %v", err)
		return
	}
	go func() {
		serveErr := grabber.Serve()
		if serveErr != nil {
			log.Errorf("Error serving dns: %v", serveErr)
		}
	}()

	httpProxyAddr := fmt.Sprintf("%s:%d",
		settings.GetHttpProxyHost(),
		settings.GetHttpProxyPort())

	flashlight.Run(httpProxyAddr, // listen for HTTP on provided address
		"127.0.0.1:0",                              // listen for SOCKS on random address
		configDir,                                  // place to store lantern configuration
		func() bool { return false },               // always connected
		func() bool { return !session.ProxyAll() }, // use shortcut
		func() bool { return false },               // not use detour
		func() bool { return false },               // do not proxy private hosts on Android
		// TODO: allow configuring whether or not to enable reporting (just like we
		// already have in desktop)
		func() bool { return true }, // auto report
		flags,
		func() bool {
			return true
		}, // beforeStart()
		func(c *client.Client) {
			clMu.Lock()
			cl = c
			clMu.Unlock()
			afterStart(session)
		},
		func(cfg *config.Global) {
			session.UpdateAdSettings(cfg.AdSettings)
			email.SetDefaultRecipient(cfg.ReportIssueEmail)
		}, // onConfigUpdate
		newUserConfig(session),
		NewStatsTracker(session),
		func(err error) {}, // onError
		session.IsProUser,
		func() string { return "" }, // only used for desktop
		func() string { return "" }, // only used for desktop
		func() bool { return settings.EnableAdBlocking() && !session.IsPlayVersion() },
		func(addr string) string {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				host = addr
			}
			ip := net.ParseIP(host)
			if ip == nil {
				log.Debugf("Unable to parse IP %v, passing through address as is", host)
				return host
			}
			updatedHost := grabber.ReverseLookup(ip)
			if updatedHost == "" {
				log.Debugf("Unable to reverse lookup %v, passing through (this shouldn't happen much)", ip)
				return addr
			}
			if splitErr != nil {
				return updatedHost
			}
			return fmt.Sprintf("%v:%v", updatedHost, port)
		},
	)
}

func bandwidthUpdates(session Session) {
	go func() {
		for quota := range bandwidth.Updates {
			session.BandwidthUpdate(getBandwidth(quota))
		}
	}()
}

func getBandwidth(quota *bandwidth.Quota) (int, int, int) {
	remaining := 0
	percent := 100
	if quota == nil {
		return 0, 0, 0
	}

	allowed := quota.MiBAllowed
	if allowed < 0 || allowed > 50000000 {
		return 0, 0, 0
	}

	if quota.MiBUsed >= quota.MiBAllowed {
		percent = 100
		remaining = 0
	} else {
		percent = int(100 * (float64(quota.MiBUsed) / float64(quota.MiBAllowed)))
		remaining = int(quota.MiBAllowed - quota.MiBUsed)
	}
	return percent, remaining, int(quota.MiBAllowed)
}

func setBandwidth(session Session) {
	percent, remaining, allowed := getBandwidth(bandwidth.GetQuota())
	if percent != 0 && remaining != 0 {
		session.BandwidthUpdate(percent, remaining, allowed)
	}
}

func afterStart(session Session) {

	bandwidthUpdates(session)

	go func() {
		if <-geolookup.OnRefresh() {
			country := geolookup.GetCountry(0)
			log.Debugf("Successful geolookup: country %s", country)
			session.SetCountry(country)
		}
	}()
}

// handleError logs the given error message
func handleError(err error) {
	log.Error(err)
}

// CheckForUpdates checks to see if a new version of Lantern is available
func CheckForUpdates() (string, error) {
	return autoupdate.CheckMobileUpdate(updateServerURL,
		common.CompileTimePackageVersion)
}

// DownloadUpdate downloads the latest APK from the given url to the apkPath
// file destination.
func DownloadUpdate(url, apkPath string, updater Updater) {
	autoupdate.UpdateMobile(url, apkPath, updater)
}
