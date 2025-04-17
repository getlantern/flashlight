package dialer

import (
	"context"
	"embed"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/x/smart"
)

type proxyless interface {
	Dialer

	status(string) int
	// reset resets the dialer to its initial state.
	reset(string)
}

type newDialerFunc func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error)

var successfulDialers sync.Map
var failed sync.Map

type proxylessDialer struct {
	newDialer   newDialerFunc
	configBytes []byte
}

func newProxylessDialer() proxyless {
	configBytes, err := configBytes()
	if err != nil {
		log.Errorf("Failed to read smart dialer config file: %v", err)
		return newFailingDialer()
	}
	return &proxylessDialer{
		newDialer: func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error) {
			strategyFinder := &smart.StrategyFinder{
				TestTimeout:  5 * time.Second,
				LogWriter:    nil,
				StreamDialer: &transport.TCPDialer{},
				PacketDialer: &transport.UDPDialer{},
			}
			return strategyFinder.NewDialer(ctx, testDomains, configBytes)
		},
		configBytes: configBytes,
	}
}

// DialContext dials out to the domain or IP address representing a destination site.
func (d *proxylessDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	deadline, _ := ctx.Deadline()
	log.Debugf("Time remaining: %v for ctx: %v", time.Until(deadline), ctx.Err())

	// We use context.Background() to create a new context with a deadline
	// because the original context may be canceled if we connect to a proxy
	// first.
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer func() {
		log.Debugf("Canceling first context for dialer for %s: %v", address, ctx.Err())
		cancel()
	}()
	// The smart dialer requires the port to be specified, so we add it if it's
	// missing. We can't do this in the dialer itself because the scheme
	// is stripped by the time the dialer is called.
	addr, host, err := normalizeAddrHost(address)
	if err != nil {
		log.Errorf("Failed to normalize address: %v", err)
		return nil, err
	}
	dialer, err := d.getOrCreateDialer(ctx, host, addr)
	if err != nil {
		d.onFailure(host, err)
		return nil, fmt.Errorf("failed to create smart dialer: %v", err)
	}

	// Store the dialer in the map right away so that we can use it for future requests.
	// If the dialer fails, we'll store a failing dialer in the map.
	d.onSuccess(host, dialer)
	streamConn, err := dialer.DialStream(ctx, addr)
	if err != nil {
		log.Errorf("❌ Failed to dial stream for %s: %v", address, err)
		d.onFailure(host, err)
		return nil, fmt.Errorf("failed to dial stream with new dialer: %v", err)
	}
	log.Debugf("✅ Successfully dialed proxyless to %s", address)
	return streamConn, nil
}

// getOrCreateDialer gets or creates a dialer for the given host.
// If a dialer already exists, it returns the existing one.
func (d *proxylessDialer) getOrCreateDialer(ctx context.Context, host, addr string) (transport.StreamDialer, error) {
	// Check if we already have a dialer for this host
	if dialer, ok := successfulDialers.Load(host); ok {
		log.Debugf("Using existing dialer for host: %s", host)
		return dialer.(transport.StreamDialer), nil
	}

	// Create a new dialer
	domains := []string{host}
	dialer, err := d.newDialer(ctx, domains, d.configBytes)
	if err != nil {
		log.Errorf("❌ Failed to create smart dialer for %v: %v", host, err)
		return nil, err
	}
	return dialer, nil
}

const (
	SUCCEEDED = iota
	FAILED
	UNKNOWN
)

func (d *proxylessDialer) onSuccess(host string, dialer transport.StreamDialer) {
	// If the dialer succeeds, we store it in the map
	successfulDialers.Store(host, dialer)
	failed.Delete(host)
	log.Debugf("Dialer succeeded for host: %s", host)
}

func (d *proxylessDialer) onFailure(host string, err error) {
	// If the dialer fails, we store it in the map
	failed.Store(host, err)
	successfulDialers.Delete(host)
	log.Debugf("Dialer failed for host: %s", host)
}

// status checks the status of the dialer for the given address.
// Returns SUCCEEDED, FAILED, or UNKNOWN.
func (d *proxylessDialer) status(address string) int {
	_, host, err := normalizeAddrHost(address)
	if err != nil {
		return FAILED
	}
	succeeded, hasSucceeded := successfulDialers.Load(host)
	failed, hasFailed := failed.Load(host)
	if hasSucceeded && succeeded != nil {
		return SUCCEEDED
	}
	if hasFailed && failed != nil {
		return FAILED
	}
	return UNKNOWN
}

// OnOptions for the state where we only have a proxyless dialer should transition to a state
// of testing the available dialers.
func (d *proxylessDialer) OnOptions(opts *Options) Dialer {
	opts.proxylessDialer = d
	return newParallelPreferProxyless(opts.proxylessDialer, newFastConnectDialer(opts))
}

// Close closes the dialer and cleans up any resources
func (d *proxylessDialer) Close() {
	// No resources to clean up
}

// reset removes the dialer associated with the given address from the map, reverting
// to an unknown state for this domain.
func (d *proxylessDialer) reset(address string) {
	reset(address)
}

func reset(address string) {
	_, host, err := normalizeAddrHost(address)
	if err != nil {
		log.Errorf("Failed to normalize address: %v", err)
		return
	}
	// Remove the dialer from the maps
	successfulDialers.Delete(host)
	failed.Delete(host)
	log.Debugf("Reset dialer for host: %s", host)
}

// normalizeAddrHost normalizes the address and host for the dialer
func normalizeAddrHost(address string) (string, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// Assume the address is missing a port and default to 443
		host = address
		port = "443"
	}
	if port != "443" && port != "" {
		return "", "", fmt.Errorf("proxyless can only dial port 443: %s", address)
	}
	addr := fmt.Sprintf("%s:%s", host, "443")
	return addr, host, nil
}

// configBytes returns the configuration bytes for the smart dialer
// It uses the embedded file system to read the configuration file
func configBytes() ([]byte, error) {
	data, err := embedFS.ReadFile("smart_dialer_config.yml")
	if err != nil {
		return nil, log.Errorf("Failed to read smart dialer config file: %v", err)
	}
	log.Debugf("Read smart dialer config file: %s", string(data))
	return data, nil
}

//go:embed smart_dialer_config.yml
var embedFS embed.FS

func newFailingDialer() proxyless {
	return &failingDialer{}
}

type failingDialer struct{}

func (d *failingDialer) status(host string) int {
	return FAILED
}

func (d *failingDialer) DialStream(ctx context.Context, addr string) (transport.StreamConn, error) {
	return nil, fmt.Errorf("intentionally failing to dial stream")
}

func (d *failingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return nil, fmt.Errorf("intentionally failing to dial stream")
}

func (d *failingDialer) Close() {
	// No resources to clean up
}

func (d *failingDialer) OnOptions(opts *Options) Dialer {
	// No options to set
	return d
}

func (d *failingDialer) reset(address string) {
	reset(address)
}
