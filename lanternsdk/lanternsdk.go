// Package lanternsdk provides a basic public SDK for embedding Lantern's circumvention capabilities
package lanternsdk

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/golog"
)

var (
	log    = golog.LoggerFor("lantern")
	runMx  sync.Mutex
	flight *flashlight.Flashlight
	cl     = eventual.NewValue()
)

// StartResult provides information about the started Lantern
type StartResult struct {
	HTTPAddr string
}

// Start starts an HTTP proxy at a random address. It blocks until the given timeout
// waiting for the proxy to listen, and returns the address at which it is listening.
// If the proxy doesn't start within the given timeout, this method returns an error.
//
// appName is a unique name for the application using the lanternsdk. It is used
// on the back-end for various purposes, including proxy assignment and performance telemetry
//
// configDir is the full path to a folder in which Lantern can save config files and
// other resources.
//
// Note - this does not wait for the entire initialization sequence to finish,
// just for the proxy to be listening. Once the proxy is listening, one can
// start to use it, even as it finishes its initialization sequence. However,
// initial activity may be slow, so clients with low read timeouts may
// time out.
//
// Note - this method gets bound to a native method in Java and Swift, so the signature
// must meet the constraints of gobind (see https://pkg.go.dev/golang.org/x/mobile/cmd/gobind).
func Start(appName, configDir, deviceID string, startTimeoutMillis int) (*StartResult, error) {
	runMx.Lock()
	defer runMx.Unlock()

	if flight == nil {
		if err := start(appName, configDir, deviceID); err != nil {
			return nil, log.Errorf("unable to start Lantern: %v", err)
		}
	}

	startTimeout := time.Duration(startTimeoutMillis) * time.Millisecond

	addr, ok := client.Addr(startTimeout)
	if !ok {
		return nil, fmt.Errorf("HTTP Proxy didn't start within %v timeout", startTimeout)
	}
	log.Debugf("Started HTTP proxy at %v", addr)

	return &StartResult{addr.(string)}, nil
}

// Stop stops the currently running Lantern if and only if it's running.
func Stop() error {
	runMx.Lock()
	defer runMx.Unlock()

	if flight == nil {
		return nil
	}
	err := flight.Stop()
	flight = nil
	return err
}

func start(appName, configDir, deviceID string) error {
	logging.EnableFileLogging(configDir)
	appdir.SetHomeDir(configDir)

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
	flight, err = flashlight.New(
		appName,
		configDir,                    // place to store lantern configuration
		false,                        // don't use VPN mode
		func() bool { return false }, // always connected
		func() bool { return false }, // don't proxy all
		func() bool { return false }, // do not proxy private hosts
		func() bool { return true },  // auto report
		flags,
		nil, // onConfigUpdate
		nil, // onProxiesUpdate
		userConfig,
		stats.NoopTracker,            // noop stats tracker
		func() bool { return false }, // isPro
		func() string { return "" },  // lang, only used for desktop
		func() string { return "" },  // adSwapTargetURL, only used for desktop
		func(addr string) (string, error) { return addr, nil }, // no dnsgrab reverse lookups on external sdk
	)
	if err != nil {
		return errors.New("Failed to start flashlight: %v", err)
	}
	go flight.Run(
		"127.0.0.1:0", // listen for HTTP on random address
		"",            // don't listen for SOCKS
		nil,           // afterStart
		nil,           // onError
	)

	return nil
}
