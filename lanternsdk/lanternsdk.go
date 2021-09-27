// Package lanternsdk provides a basic public SDK for embedding Lantern's circumvention capabilities
package lanternsdk

import (
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/golog"
)

var (
	log     = golog.LoggerFor("lantern")
	runMx   sync.Mutex
	running bool
	cl      = eventual.NewValue()
)

// ProxyAddr provides information about the started Lantern
type ProxyAddr struct {
	HTTPAddr string
	HTTPHost string
	HTTPPort int
}

// Gets the current proxy address, waiting up to timeoutMillis for the proxy to start up if necessary.
//
// Note - this does not wait for the entire initialization sequence to finish,
// just for the proxy to be listening. Once the proxy is listening, one can
// start to use it, even as it finishes its initialization sequence. However,
// initial activity may be slow, so clients with low read timeouts may
// time out.
//
func GetProxyAddr(timeoutMillis int) (*ProxyAddr, error) {
	startTime := time.Now()
	remainingTimeout := func() time.Duration {
		return time.Now().Add(time.Duration(timeoutMillis) * time.Millisecond).Sub(startTime)
	}

	addr, ok :=
		client.Addr(remainingTimeout())
	if !ok {
		return nil, log.Error("HTTP Proxy didn't start in time")
	}
	log.Debugf("Using HTTP proxy at %v", addr)

	// wait for geolookup to complete so that we don't run into
	// "proxywhitelistedonly enabled because geolookup has not yet finished"
	country := geolookup.GetCountry(remainingTimeout())
	if country == "" {
		return nil, log.Error("failed to complete geolookup in time")
	}

	httpAddr := addr.(string)
	host, portString, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return nil, log.Errorf("failed to split host:port from %v: %v", httpAddr, err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return nil, log.Errorf("failed to parse integer from port %v: %v", portString)
	}

	return &ProxyAddr{
		HTTPAddr: httpAddr,
		HTTPHost: host,
		HTTPPort: port,
	}, nil
}

// Start starts an HTTP proxy at a random address. It blocks until the given timeout
// waiting for the proxy to listen, and returns the address at which it is listening.
// If the proxy doesn't start within the given timeout, this method returns an error.
//
// appName is a unique name for the application using the lanternsdk. It is used
// on the back-end for various purposes, including proxy assignment and performance telemetry
//
// If proxyAll is true, Lantern will proxy all traffic. If false, it will only proxy whitelisted
// domains or traffic to domains that appear to be blocked.
//
// Note - this method gets bound to a native method in Java and Swift, so the signature
// must meet the constraints of gobind (see https://pkg.go.dev/golang.org/x/mobile/cmd/gobind).
func Start(appName, configDir, deviceID string, proxyAll bool) error {
	runMx.Lock()
	defer runMx.Unlock()

	if !running {
		if err := start(appName, configDir, deviceID, proxyAll); err != nil {
			return log.Errorf("unable to start Lantern: %v", err)
		}
		running = true
	}

	return nil
}

func start(appName, configDir, deviceID string, proxyAll bool) error {
	logging.EnableFileLogging(configDir)
	appdir.SetHomeDir(configDir)
	increaseFilesLimit()

	log.Debugf("Starting lantern: configDir %s", configDir)
	flags := map[string]interface{}{
		"borda-report-interval":   5 * time.Minute,
		"borda-sample-percentage": float64(0.01),
		"staging":                 common.Staging,
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.New("Unable to create configDir at %v: %v", configDir, err)
	}

	userConfig := &common.UserConfigData{
		DeviceID: deviceID,
		UserID:   0,       // not used for sdk clients
		Token:    "",      // not used for sdk clients
		Language: "en_US", // specific language doesn't really matter since we're not handling UI
	}

	runner, err := flashlight.New(
		appName,
		configDir,                       // place to store lantern configuration
		false,                           // don't use VPN mode
		func() bool { return false },    // always connected
		func() bool { return proxyAll }, // whetther to proxy all
		func() bool { return false },    // don't inject google ads
		func() bool { return false },    // do not proxy private hosts
		func() bool { return true },     // auto report
		flags,
		nil, // onConfigUpdate
		nil, // onProxiesUpdate
		userConfig,
		stats.NewTracker(),
		func() bool { return false }, // isPro
		func() string { return "" },  // lang, only used for desktop
		func() string { return "" },  // adSwapTargetURL, only used for desktop
		func(addr string) (string, error) { return addr, nil }, // no dnsgrab reverse lookups on external sdk
		func() string { return "" },                            // ad url, only used for desktop
		func(category, action, label string) {},                // no event tracking, only on desktop
	)
	if err != nil {
		return errors.New("Failed to start flashlight: %v", err)
	}
	go runner.Run(
		"127.0.0.1:0", // listen for HTTP on random address
		"127.0.0.1:0", // listen for SOCKS on random address
		nil,           // afterStart
		nil,           // onError
	)
	return nil
}
