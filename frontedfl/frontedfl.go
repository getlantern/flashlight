// Package frontedfl is used to configure and interface with github.com/getlantern/fronted for use
// in the Flashlight client.
package frontedfl

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
)

// CloudFront is the only provider we use.
const defaultProviderID = "cloudfront"

var defaultConfig eventual.Value // type Config

// Config represents configuration for creating fronted round trippers.
type Config struct {
	Providers   map[string]*fronted.Provider
	CertPool    *x509.CertPool
	CacheFolder string
}

// NewRoundTripper initializes a round tripper using the configuration. Blocks until a RoundTripper
// is ready (see fronted.NewRoundTripper). Will return an error if the context expires before the
// RoundTripper can be returned.
//
// Fields set in opts will override anything in cfg. If opts.CacheFile is a relative path, it will
// be interpreted to reside with cfg.CacheFolder.
func (cfg Config) NewRoundTripper(ctx context.Context, opts fronted.RoundTripperOptions) (fronted.RoundTripper, error) {
	if opts.CertPool == nil {
		opts.CertPool = cfg.CertPool
	}
	if opts.CacheFile != "" && !filepath.IsAbs(opts.CacheFile) {
		opts.CacheFile = filepath.Join(cfg.CacheFolder, opts.CacheFile)
	}
	if opts.CacheFile == "" {
		opts.CacheFile = filepath.Join(cfg.CacheFolder, "masquerade_cache")
	}

	// CloudFront only for wss.
	p := cfg.Providers[defaultProviderID]
	if p == nil {
		return nil, errors.New("no CloudFront provider in global config (only CloudFront is used)")
	}
	p = fronted.NewProvider(p.HostAliases, p.TestURL, p.Masquerades, p.Validator, []string{"*.cloudfront.net"})
	pOnly := map[string]*fronted.Provider{defaultProviderID: p}

	return fronted.NewRoundTripperContext(ctx, pOnly, defaultProviderID, fronted.RoundTripperOptions{})
}

// SetDefaults sets fronting defaults for global usage. Calls to NewRoundTripper will use this
// configuration and will block until this function is called.
func SetDefaults(cfg Config) {
	defaultConfig.Set(cfg)
}

// NewRoundTripper initializes a round tripper using the configured defaults. Blocks until
// SetDefaults is called. Also blocks until the RoundTripper is ready (see fronted.NewRoundTripper).
// Will return an error if the context expires before the RoundTripper can be returned.
//
// Fields set in opts will override anything provided to SetDefaults. If opts.CacheFile is a
// relative path, it will be interpreted to reside within the globally-configured cache folder.
func NewRoundTripper(ctx context.Context, opts fronted.RoundTripperOptions) (fronted.RoundTripper, error) {
	cfg, err := getEventualContext(ctx, defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to obtain global config: %w", err)
	}
	return cfg.(Config).NewRoundTripper(ctx, opts)
}

// Essentially creating our own v.Get(ctx) function, instead of v.Get(timeout).
func getEventualContext(ctx context.Context, v eventual.Value) (interface{}, error) {
	// The default timeout is large enough that it shouldn't supercede the context timeout, but
	// short enough that we'll eventually clean up launched routines.
	const defaultTimeout = time.Hour

	timeout := defaultTimeout
	if deadline, ok := ctx.Deadline(); ok {
		// Add 10s to the context timeout to ensure ctx.Done() closes before v.Get times out.
		timeout = time.Until(deadline) + 10*time.Second
	}
	resultCh := make(chan interface{})
	go func() {
		result, _ := v.Get(timeout)
		resultCh <- result
	}()
	select {
	case r := <-resultCh:
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
