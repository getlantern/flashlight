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
}

func newProxylessDialer() Dialer {
	return &proxylessDialer{
		strategyFinder: &smart.StrategyFinder{
			TestTimeout:  5 * time.Second,
			LogWriter:    nil,
			StreamDialer: &transport.TCPDialer{},
			PacketDialer: &transport.UDPDialer{},
		},
	}
}

// DialContext dials out to the domain or IP address representing a destination site.
func (d *proxylessDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Check if the dialer already exists in the map
	if dialer, ok := d.dialers.Load(addr); ok {
		// If it exists, use it
		return dialer.(transport.StreamDialer).DialStream(ctx, addr)
	}
	// If it doesn't exist, create a new dialer
	// Parse the domain from the address
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	domains := []string{host}

	configBytes, err := embedFS.ReadFile("smart_dialer_config.yml")
	if err != nil {
		log.Errorf("Failed to read smart dialer config: %v", err)
		return nil, err
	}
	dialer, err := d.strategyFinder.NewDialer(context.Background(), domains, configBytes)
	if err != nil {
		log.Errorf("Failed to create smart dialer %v", err)
		return nil, err
	}
	// Store the dialer in the map right away so that we can use it for future requests.
	// If the dialer fails, we'll store a failing dialer in the map.
	d.dialers.Store(addr, dialer)

	streamConn, err := dialer.DialStream(ctx, addr)
	if err != nil {
		d.dialers.Store(addr, newFailingDialer())
		return nil, fmt.Errorf("failed to dial stream: %v", err)
	}
	return streamConn, nil
}

// Close closes the dialer and cleans up any resources
func (d *proxylessDialer) Close() {
	// No resources to clean up
}

//go:embed smart_dialer_config.yml
var embedFS embed.FS

// newFailingDialer returns a dialer that always fails to connect
func newFailingDialer() transport.StreamDialer {
	return &failingDialer{}
}

type failingDialer struct{}

func (d *failingDialer) DialStream(ctx context.Context, addr string) (transport.StreamConn, error) {
	return nil, fmt.Errorf("failed to dial stream")
}
