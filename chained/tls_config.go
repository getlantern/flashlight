package chained

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/hellocap"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/tlsresumption"
)

// Generates TLS configuration for connecting to proxy specified by the ChainedServerInfo. This
// function may block while determining things like how to mimic the default browser's client hello.
//
// Returns a slice of ClientHellos to be used for dialing. These hellos are in priority order: the
// first hello is the "ideal" one and the remaining hellos serve as backup in case something is
// wrong with the previous hellos. There will always be at least one hello. For each hello, the
// ClientHelloSpec will be non-nil if and only if the ClientHelloID is tls.HelloCustom.
func tlsConfigForProxy(configDir string, ctx context.Context, name string, s *ChainedServerInfo, uc common.UserConfig) (
	*tls.Config, []hello) {

	configuredHelloID := s.clientHelloID()
	var ss *tls.ClientSessionState
	var err error
	if s.TLSClientSessionState != "" {
		ss, err = tlsresumption.ParseClientSessionState(s.TLSClientSessionState)
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

	var configuredHelloSpec *tls.ClientHelloSpec
	if configuredHelloID == helloBrowser {
		configuredHelloID, configuredHelloSpec = getBrowserHello(configDir, ctx, uc)
	}

	sessionTTL := simbrowser.ChooseForUser(ctx, uc).SessionTicketLifetime
	sessionCache := newExpiringSessionCache(name, sessionTTL, ss)
	cipherSuites := orderedCipherSuitesFromConfig(s)

	cfg := &tls.Config{
		ClientSessionCache: sessionCache,
		CipherSuites:       cipherSuites,
		ServerName:         s.TLSServerNameIndicator,
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}
	hellos := []hello{
		{configuredHelloID, configuredHelloSpec},
		{tls.HelloChrome_Auto, nil},
		{tls.HelloGolang, nil},
	}

	return cfg, hellos
}

// getBrowserHello determines the best way to mimic the system's default web browser. There are a
// few possible failure points in making this determination, e.g. a failure to obtain the default
// browser or a failure to capture a hello from the browser. However, this function will always find
// something reasonable to fall back on.
func getBrowserHello(configDir string, ctx context.Context, uc common.UserConfig) (tls.ClientHelloID, *tls.ClientHelloSpec) {
	// We have a number of ways to approximate the browser's ClientHello format. We begin with the
	// most desirable, progressively falling back to less desirable options on failure.

	op := ops.Begin("get_browser_hello")
	op.Set("platform", runtime.GOOS)
	defer op.End()

	helloSpec, err := activelyObtainBrowserHello(configDir, ctx)
	if err == nil {
		return tls.HelloCustom, helloSpec
	}
	op.FailIf(err)
	log.Debugf("failed to actively obtain browser hello: %v", err)

	// Our last option is to simulate a browser choice for the user based on market share.
	return simbrowser.ChooseForUser(ctx, uc).ClientHelloID, nil
}

func activelyObtainBrowserHello(configDir string, ctx context.Context) (*tls.ClientHelloSpec, error) {
	const tlsRecordHeaderLen = 5

	activeCaptureHelloCache := helloCacheInConfigDir(configDir, "hello-cache.active-capture")

	helloSpec, err := activeCaptureHelloCache.readAndParse()
	if err == nil {
		return helloSpec, nil
	}
	log.Debugf("failed to read actively obtained hello from cache: %v", err)

	sampleHello, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	helloSpec, err = tls.FingerprintClientHello(sampleHello[tlsRecordHeaderLen:])
	if err != nil {
		return nil, fmt.Errorf("failed to fingerprint sample hello: %w", err)
	}
	if err := activeCaptureHelloCache.write(sampleHello); err != nil {
		log.Debugf("failed to write actively obtained hello to cache: %v", err)
	}
	return helloSpec, nil
}

func orderedCipherSuitesFromConfig(s *ChainedServerInfo) []uint16 {
	if common.Platform == "android" {
		return s.mobileOrderedCipherSuites()
	}
	return s.desktopOrderedCipherSuites()
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

type helloCacheFile struct {
	filename string
	initErr  error
}

func helloCacheInConfigDir(configDir string, relativeFilename string) helloCacheFile {
	absoluteFilename, err := common.InConfigDir(configDir, relativeFilename)
	return helloCacheFile{absoluteFilename, err}
}

func (f helloCacheFile) write(hello []byte) error {
	if f.initErr != nil {
		return fmt.Errorf("cache initilization failed: %w", f.initErr)
	}
	return ioutil.WriteFile(f.filename, hello, 0644)
}

func (f helloCacheFile) readAndParse() (*tls.ClientHelloSpec, error) {
	if f.initErr != nil {
		return nil, fmt.Errorf("cache initilization failed: %w", f.initErr)
	}
	hello, err := ioutil.ReadFile(f.filename)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	spec, err := tls.FingerprintClientHello(hello)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file contents: %w", err)
	}
	return spec, nil
}
