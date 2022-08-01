package chained

import (
	"context"
	"io"
	"os"
	"runtime"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/common/apipb"
	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/tlsresumption"
)

// Generates TLS configuration for connecting to proxy specified by the apipb.ProxyConfig. This
// function may block while determining things like how to mimic the default browser's client hello.
//
// Returns a slice of ClientHellos to be used for dialing. These hellos are in priority order: the
// first hello is the "ideal" one and the remaining hellos serve as backup in case something is
// wrong with the previous hellos. There will always be at least one hello. For each hello, the
// ClientHelloSpec will be non-nil if and only if the ClientHelloID is tls.HelloCustom.
func tlsConfigForProxy(ctx context.Context, configDir, proxyName string, pc *apipb.ProxyConfig, uc common.UserConfig) (
	*tls.Config, []helloSpec) {

	configuredHelloID := clientHelloID(pc)
	var ss *tls.ClientSessionState
	var err error
	if pc.TLSClientSessionState != "" {
		ss, err = tlsresumption.ParseClientSessionState(pc.TLSClientSessionState)
		if err != nil {
			log.Errorf("Unable to parse serialized client session state, continuing with normal handshake: %v", err)
		} else {
			log.Debug("Using serialized client session state")
			if configuredHelloID.Client == "Golang" {
				log.Debug("Need to mimic browser hello for session resumption, defaulting to HelloChrome_Auto")
				configuredHelloID = tls.HelloChrome_Auto
			}
		}
	}

	configuredHelloSpec := helloSpec{configuredHelloID, nil}
	if configuredHelloID == helloBrowser {
		configuredHelloSpec = getBrowserHello(ctx, configDir, uc)
	}

	sessionTTL := simbrowser.ChooseForUser(ctx, uc).SessionTicketLifetime
	sessionCache := newExpiringSessionCache(proxyName, sessionTTL, ss)

	// We configure cipher suites here, then specify later which ClientHello type to mimic. As the
	// ClientHello type itself specifies cipher suites, it's not immediately obvious who wins.
	//
	// When we specify HelloGolang (a fallback at the time of this writing), the cipher suites in
	// the config are used. For our purposes, this is preferable, particularly as we can configure
	// these remotely.
	//
	// For all other hello types, the cipher suite in the ClientHelloSpec are used:
	//  1. With HelloCustom, we call ApplyPreset manually; skip to step 4.
	//  2. The handshake is built by BuildHandshakeState, which calls applyPresetByID:
	//     https://github.com/getlantern/utls/blob/1abdc4b1acab98e8776ae9a5201f67968ffa01dc/u_conn.go#L82
	//  3. For HelloRandomized*, random suites are generated by generateRandomSpec. For other,
	//     non-custom hellos, ApplyPreset is called:
	//     https://github.com/getlantern/utls/blob/1abdc4b1acab98e8776ae9a5201f67968ffa01dc/u_parrots.go#L977
	//  4. ApplyPreset configures the cipher suites according to the ClientHelloSpec:
	//     https://github.com/getlantern/utls/blob/1abdc4b1acab98e8776ae9a5201f67968ffa01dc/u_parrots.go#L1034-L1040

	cipherSuites := orderedCipherSuitesFromConfig(pc)

	cfg := &tls.Config{
		ClientSessionCache: sessionCache,
		CipherSuites:       cipherSuites,
		ServerName:         pc.TLSServerNameIndicator,
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}
	hellos := []helloSpec{
		configuredHelloSpec,
		{tls.HelloChrome_Auto, nil},
		{tls.HelloGolang, nil},
	}

	return cfg, hellos
}

// getBrowserHello determines the best way to mimic the system's default web browser. There are a
// few possible failure points in making this determination, e.g. a failure to obtain the default
// browser or a failure to capture a hello from the browser. However, this function will always find
// something reasonable to fall back on.
func getBrowserHello(ctx context.Context, configDir string, uc common.UserConfig) helloSpec {
	// We have a number of ways to approximate the browser's ClientHello format. We begin with the
	// most desirable, progressively falling back to less desirable options on failure.

	op := ops.Begin("get_browser_hello")
	op.Set("platform", runtime.GOOS)
	defer op.End()

	hello, err := activelyObtainBrowserHello(ctx, configDir)
	if err == nil {
		return *hello
	}
	op.FailIf(err)
	log.Debugf("failed to actively obtain browser hello: %v", err)

	// Our last option is to simulate a browser choice for the user based on market share.
	return helloSpec{simbrowser.ChooseForUser(ctx, uc).ClientHelloID, nil}
}

func orderedCipherSuitesFromConfig(pc *apipb.ProxyConfig) []uint16 {
	if common.Platform == "android" {
		return mobileOrderedCipherSuites(pc)
	}
	return desktopOrderedCipherSuites(pc)
}

// Write the session keys to file if SSLKEYLOGFILE is set, same as browsers.
func getTLSKeyLogWriter() io.Writer {
	createKeyLogWriterOnce.Do(func() {
		path := os.Getenv("SSLKEYLOGFILE")
		if path == "" {
			return
		}
		var err error
		tlsKeyLogWriter, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			log.Debugf("Error creating keylog file at %v: %s", path, err)
		}
	})
	return tlsKeyLogWriter
}
