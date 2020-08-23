package chained

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/flashlight/hellocap"
	"github.com/getlantern/tlsresumption"
)

// TODO: delete [3349] log statements

var (
	activeCaptureHelloCache = helloCacheInConfigDir("hello-cache.active-capture")
)

// Generates TLS configuration for connecting to proxy specified by the ChainedServerInfo. This
// function may block while determining things like how to mimic the default browser's client hello.
//
// The ClientHelloSpec will be non-nil if and only if the ClientHelloID is tls.HelloCustom.
func tlsConfigForProxy(ctx context.Context, name string, s *ChainedServerInfo, uc common.UserConfig) (
	*tls.Config, tls.ClientHelloID, *tls.ClientHelloSpec) {

	helloID := s.clientHelloID()
	var ss *tls.ClientSessionState
	var err error
	if s.TLSClientSessionState != "" {
		ss, err = tlsresumption.ParseClientSessionState(s.TLSClientSessionState)
		if err != nil {
			log.Errorf("Unable to parse serialized client session state, continuing with normal handshake: %v", err)
		} else {
			log.Debug("Using serialized client session state")
			if helloID.Client == "Golang" {
				log.Debug("Need to mimic browser hello for session resumption, defaulting to HelloChrome_Auto")
				helloID = tls.HelloChrome_Auto
			}
		}
	}

	var helloSpec *tls.ClientHelloSpec
	if helloID == helloBrowser {
		log.Debug("[3349] obtaining browser hello spec")
		helloSpec = getBrowserHello(ctx, s, uc)
		helloID = tls.HelloCustom
	} else {
		log.Debug("[3349] not using helloBrowser")
	}

	sessionTTL := simbrowser.ChooseForUser(ctx, uc).SessionTicketLifetime()
	sessionCache := newExpiringSessionCache(name, sessionTTL, ss)
	cipherSuites := orderedCipherSuitesFromConfig(s)

	cfg := &tls.Config{
		ClientSessionCache: sessionCache,
		CipherSuites:       cipherSuites,
		ServerName:         s.TLSServerNameIndicator,
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}

	return cfg, helloID, helloSpec
}

// getBrowserHello determines the best way to mimic the system's default web browser. There are a
// few possible failure points in making this determination, e.g. a failure to obtain the default
// browser or a failure to capture a hello from the browser. However, this function will always find
// something reasonable to fall back on.
func getBrowserHello(ctx context.Context, s *ChainedServerInfo, uc common.UserConfig) *tls.ClientHelloSpec {
	// We have a number of ways to approximate the browser's ClientHello format. We begin with the
	// most desirable, progressively falling back to less desirable options on failure.

	log.Debugf("[3349] obtaining browser hello spec for SNI '%s'", s.TLSServerNameIndicator)

	// TODO: determine if it's really necessary to get hello for domain because the timing is difficult
	// Passively capturing the hello for the domain would be tricky too

	helloSpec, err := activelyObtainBrowserHello(ctx, s.TLSServerNameIndicator)
	if err == nil {
		return helloSpec
	}
	log.Debugf("[3349] failed to actively obtain browser hello: %v", err) // TODO: remove [3349], keep log

	// Our last option is to simulate a browser choice for the user based on market share.
	simulatedHelloSpec := simbrowser.ChooseForUser(ctx, uc).ClientHelloSpec()
	return &simulatedHelloSpec
}

func activelyObtainBrowserHello(ctx context.Context, sni string) (*tls.ClientHelloSpec, error) {
	log.Debugf("[3349] reading from cache")
	helloSpec, err := activeCaptureHelloCache.readAndParse()
	if err == nil {
		log.Debugf("[3349] read hello from cache")
		return helloSpec, nil
	}
	log.Debugf("[3349] failed to read actively obtained hello from cache: %v", err) // TODO: remove [3349], keep log

	log.Debug("[3349] waiting for domain routing to be configured")
	// Domain routing must be configured before we can use our domain mapper.
	if err := domainrouting.WaitForConfigure(ctx); err != nil {
		return nil, fmt.Errorf("domain routing was not configured in time: %w", err)
	}
	log.Debug("[3349] domain routing configured")

	sampleHello, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	helloSpec, err = tls.FingerprintClientHello(sampleHello)
	if err != nil {
		return nil, fmt.Errorf("failed to fingerprint sample hello: %w", err)
	}
	if err := activeCaptureHelloCache.write(sampleHello); err != nil {
		log.Debugf("[3349] failed to write actively obtained hello to cache: %v", err) // TODO: remove [3349], keep log
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

func helloCacheInConfigDir(relativeFilename string) helloCacheFile {
	absoluteFilename, err := common.InConfigDir("", relativeFilename)
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
