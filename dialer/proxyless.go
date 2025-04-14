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

type proxylessDialer struct {
	strategyFinder *smart.StrategyFinder
	dialers        sync.Map
	configBytes    []byte
}

func newProxylessDialer() Dialer {
	configBytes, err := configBytes()
	if err != nil {
		log.Errorf("Failed to read smart dialer config file: %v", err)
		return newFailingDialer()
	}
	return &proxylessDialer{
		strategyFinder: &smart.StrategyFinder{
			TestTimeout:  5 * time.Second,
			LogWriter:    nil,
			StreamDialer: &transport.TCPDialer{},
			PacketDialer: &transport.UDPDialer{},
		},
		configBytes: configBytes,
	}
}

// DialContext dials out to the domain or IP address representing a destination site.
func (d *proxylessDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// The smart dialer requires the port to be specified, so we add it if it's
	// missing. We can't do this in the dialer itself because the scheme
	// is stripped by the time the dialer is called.
	var addr string
	var host string
	var port string
	var err error
	if host, port, err = net.SplitHostPort(address); err != nil {
		addr = fmt.Sprintf("%s:%s", host, "443")
	} else if port == "" || port == "443" {
		addr = fmt.Sprintf("%s:%s", host, "443")
	} else {
		return nil, fmt.Errorf("proxyless can only dial 443: %s", addr)
	}
	// Check if the dialer already exists in the map
	if dialer, ok := d.dialers.Load(host); ok {
		// If it exists, use it
		return dialer.(transport.StreamDialer).DialStream(ctx, address)
	}

	domains := []string{host}
	dialer, err := d.strategyFinder.NewDialer(context.Background(), domains, d.configBytes)
	if err != nil {
		log.Errorf("Failed to create smart dialer %v", err)
		return nil, err
	}
	// Store the dialer in the map right away so that we can use it for future requests.
	// If the dialer fails, we'll store a failing dialer in the map.
	d.dialers.Store(host, dialer)

	streamConn, err := dialer.DialStream(ctx, addr)
	if err != nil {
		d.dialers.Store(host, newFailingStreamDialer())
		return nil, fmt.Errorf("failed to dial stream: %v", err)
	}
	return streamConn, nil
}

// Close closes the dialer and cleans up any resources
func (d *proxylessDialer) Close() {
	// No resources to clean up
}

// configBytes returns the configuration bytes for the smart dialer
// It uses the embedded file system to read the configuration file
func configBytes() ([]byte, error) {
	if configBytes, err := embedFS.ReadFile("smart_dialer_config.yml"); err != nil {
		return nil, log.Errorf("Failed to read smart dialer config file: %v", err)
	} else {
		return configBytes, nil
	}
}

//go:embed smart_dialer_config.yml
var embedFS embed.FS

// newFailingStreamDialer returns a dialer that always fails to connect
func newFailingStreamDialer() transport.StreamDialer {
	return &failingDialer{}
}

func newFailingDialer() Dialer {
	return &failingDialer{}
}

type failingDialer struct{}

func (d *failingDialer) DialStream(ctx context.Context, addr string) (transport.StreamConn, error) {
	return nil, fmt.Errorf("failed to dial stream")
}

func (d *failingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return nil, fmt.Errorf("failed to dial context")
}

func (d *failingDialer) Close() {
	// No resources to clean up
}
