// Package android implements the mobile application functionality of flashlight
package android

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/autoupdate"
	"github.com/getlantern/bandwidth"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/proxied"
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
	dialers         = []balancer.Dialer{}

	surveyHTTPClient = &http.Client{
		Transport: proxied.ChainedThenFrontedWith("d38rvu630khj2q.cloudfront.net", ""),
	}

	surveyURL = "https://raw.githubusercontent.com/getlantern/loconf/master/ui.json"

	startOnce sync.Once
)

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
	netx.OverrideResolve(p.Resolve)
	for _, dialer := range dialers {
		if dialer.KCPEnabled() {
			log.Debugf("Enabling KCP for dialer: %s", dialer.Label())
			// refresh KCP connections
			dialer.RefreshKCP()
		}
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
		return nil, fmt.Errorf("HTTP Proxy didn't start within given timeout")
	}

	socksAddr, ok := client.Socks5Addr((time.Duration(settings.TimeoutMillis()) * time.Millisecond) - elapsed())
	if !ok {
		err := fmt.Errorf("SOCKS5 Proxy didn't start within given timeout")
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
		settings.DnsGrabServer(),
		settings.DefaultDnsServer())
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

	flashlight.Run("127.0.0.1:0", // listen for HTTP on random address
		"127.0.0.1:0",                // listen for SOCKS on random address
		configDir,                    // place to store lantern configuration
		func() bool { return false }, // always connected
		// TODO: allow configuring whether or not to enable shortcut depends on
		// proxyAll option (just like we already have in desktop)
		func() bool { return !session.ProxyAll() }, // use shortcut
		func() bool { return false },               // not use detour
		// TODO: allow configuring whether or not to enable reporting (just like we
		// already have in desktop)
		func() bool { return true }, // on Android, we allow private hosts
		func() bool { return true }, // auto report
		flags,
		func() bool {
			return true
		}, // beforeStart()
		func() {
			afterStart(session)
		}, // afterStart()
		func(cfg *config.Global) {
		}, // onConfigUpdate
		session,
		NewStatsTracker(session),
		func(err error) {}, // onError
		session.GetDeviceID(),
		func(ds []balancer.Dialer) {
			if len(ds) == 0 {
				log.Error("No new dialers")
				return
			}
			log.Debugf("Adding %d new dialers", len(dialers))
			dialers = make([]balancer.Dialer, len(ds))
			copy(dialers, ds)
		},
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
		// Request filter for HTTP proxy. Currently only used on desktop.
		func(r *http.Request) (*http.Request, error) { return r, nil },
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

func initSession(session Session) {
	if session.GetUserID() == 0 {
		// create new user first if we have no valid user id
		_, err := newUser(newRequest(session))
		if err != nil {
			log.Errorf("Could not create new pro user")
			return
		}
	}

	log.Debugf("New Lantern session with user id %d", session.GetUserID())

	setBandwidth(session)
	setSurvey(session)

	req := newRequest(session)

	for _, proFn := range []proFunc{plans, userData} {
		_, err := proFn(req)
		if err != nil {
			log.Errorf("Error making pro request: %v", err)
		}
	}
}

func afterStart(session Session) {

	bandwidthUpdates(session)

	go initSession(session)

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

func extractUrl(surveys map[string]*json.RawMessage, locale string) (string, error) {

	var survey SurveyInfo
	var err error
	if val, ok := surveys[locale]; ok {
		err = json.Unmarshal(*val, &survey)
		if err != nil {
			handleError(fmt.Errorf("Error parsing survey: %v", err))
			return "", err
		}
		if !survey.Enabled {
			log.Debugf("Survey %s is disabled for locale: %s", survey.Url, locale)
			return "", nil
		}

		if rand.Float64() >= survey.Probability {
			log.Debugf("Not electing to show survey based on probability field")
			return "", nil
		}

		log.Debugf("Found a survey for locale %s: %s", locale, survey.Url)
		return survey.Url, nil
	} else if locale != defaultLocale {
		log.Debugf("No survey found for %s ; Using default locale: %s", locale, defaultLocale)
		return extractUrl(surveys, defaultLocale)
	}
	return "", nil
}

func setSurvey(session Session) {
	url, err := surveyRequest(session.Locale())
	if err == nil && url != "" {
		log.Debugf("Setting survey url to %s", url)
		session.ShowSurvey(url)
	}
}

func surveyRequest(locale string) (string, error) {
	var err error
	var req *http.Request
	var res *http.Response

	var surveyResp map[string]*json.RawMessage

	if req, err = http.NewRequest("GET", surveyURL, nil); err != nil {
		handleError(fmt.Errorf("Error fetching survey: %v", err))
		return "", err
	}

	if res, err = surveyHTTPClient.Do(req); err != nil {
		handleError(fmt.Errorf("Error fetching feed: %v", err))
		return "", err
	}

	defer res.Body.Close()

	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		handleError(fmt.Errorf("Error reading survey: %v", err))
		return "", err
	}

	log.Debugf("Survey response: %s", string(contents))

	err = json.Unmarshal(contents, &surveyResp)
	if err != nil {
		handleError(fmt.Errorf("Error parsing survey: %v", err))
		return "", err
	}

	if surveyResp["survey"] != nil {
		var surveys map[string]*json.RawMessage
		err = json.Unmarshal(*surveyResp["survey"], &surveys)
		if err != nil {
			handleError(fmt.Errorf("Error parsing survey: %v", err))
			return "", err
		}
		locale = strings.Replace(locale, "_", "-", -1)
		return extractUrl(surveys, locale)
	}
	log.Errorf("Error parsing survey response: missing from map")
	return "", nil
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
