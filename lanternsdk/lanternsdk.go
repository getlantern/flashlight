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
	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/golog"
)

var (
	log    = golog.LoggerFor("lantern")
	runMx  sync.Mutex
	runner *flashlight.Flashlight
)

// ProxyAddr provides information about the address at which Lantern is listening
type ProxyAddr struct {
	HTTPAddr string
	HTTPHost string
	HTTPPort int
}

// Start starts an HTTP proxy at a random address. It blocks until the given timeout
// waiting for the proxy to listen, and returns the address at which it is listening.
// If the proxy doesn't start within the given timeout, this method returns an error.
//
// Every time that you call Start(), it will return a new HTTP proxy address. This is useful on
// iOS, where a sleep/wake cycle puts the HTTP proxy's listener socket into a not connected
// state that returns ENOTCONN to clients. Go doesn't notice that the socket is no longer
// connected and blocks on Accept(). Calling Start() at this point closes the old listener
// and starts listening on a new socket (with a different address).
//
// appName is a unique name for the application using the lanternsdk. It is used
// on the back-end for various purposes, including proxy assignment and performance telemetry
//
// If proxyAll is true, Lantern will proxy all traffic. If false, it will only proxy whitelisted
// domains or traffic to domains that appear to be blocked.
//
// Note - this does not wait for the entire initialization sequence to finish,
// just for the proxy to be listening. Once the proxy is listening, one can
// start to use it, even as it finishes its initialization sequence. However,
// initial activity may be slow, so clients with low read timeouts may
// time out.
//
// Note - this method gets bound to a native method in Java and Swift, so the signature
// must meet the constraints of gobind (see https://pkg.go.dev/golang.org/x/mobile/cmd/gobind).
func Start(appName, configDir, deviceID string, proxyAll bool, startTimeoutMillis int) (*ProxyAddr, error) {
	runMx.Lock()
	defer runMx.Unlock()

	startTime := time.Now()
	remainingTimeout := func() time.Duration {
		return time.Now().Add(time.Duration(startTimeoutMillis) * time.Millisecond).Sub(startTime)
	}

	if err := start(appName, configDir, deviceID, proxyAll); err != nil {
		return nil, log.Errorf("unable to start Lantern: %v", err)
	}

	addr, ok := client.Addr(remainingTimeout())
	if !ok {
		return nil, log.Error("HTTP Proxy didn't start in time")
	}
	log.Debugf("Started HTTP proxy at %v", addr)

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

func start(appName, configDir, deviceID string, proxyAll bool) error {
	if runner != nil {
		log.Debug("Stopping running Lantern")
		runner.Stop()
	} else {
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

		var err error
		runner, err = flashlight.New(
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
		runner.StartBackgroundServices()
	}

	log.Debug("Running lantern")
	go runner.RunClientListeners(
		"127.0.0.1:0", // listen for HTTP on random address
		"",            // don't listen for SOCKS
		nil,           // afterStart
		nil,           // onError
	)
	return nil
}
