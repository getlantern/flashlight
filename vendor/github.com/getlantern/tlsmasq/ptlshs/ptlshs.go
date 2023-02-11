// Package ptlshs implements proxied TLS handshakes. When a client dials a ptlshs listener, the
// initial handshake is proxied to another TLS server. When this proxied handshake is complete, the
// dialer signals to the listener and the connection is established. From here, another, "true"
// handshake may be performed, but this is not the purvue of the ptlshs package.
package ptlshs

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"
)

const (
	// DefaultNonceTTL is used when DialerConfig.NonceTTL is not specified.
	DefaultNonceTTL = 30 * time.Minute

	// DefaultNonceSweepInterval is used when ListenerConfig.NonceSweepInterval is not specified.
	DefaultNonceSweepInterval = time.Minute
)

// This should be plenty large enough for handshake records. In the event that the connection
// becomes a fully proxied connection, we may split records up, but that's not a problem.
const listenerReadBufferSize = 1024

// A Secret pre-shared between listeners and dialers. This is used to secure the completion signal
// sent by the dialer.
type Secret [52]byte

// HandshakeResult is the result of a TLS handshake.
type HandshakeResult struct {
	Version, CipherSuite uint16
}

// Handshaker executes a TLS handshake using the input connection as a transport. Handshakers must
// unblock and return an error if the connection is closed. Usually this does not require special
// handling as the connection's I/O functions should do the same.
type Handshaker interface {
	Handshake(net.Conn) (*HandshakeResult, error)
}

// StdLibHandshaker execute a TLS handshakes using the Go standard library.
type StdLibHandshaker struct {
	// Config for the handshake. May not be nil. ServerName should be set according to the origin
	// server.
	Config *tls.Config
}

// Handshake executes a TLS handshake using conn as a transport.
func (h StdLibHandshaker) Handshake(conn net.Conn) (*HandshakeResult, error) {
	if h.Config == nil {
		return nil, errors.New("std lib handshaker config must not be nil")
	}
	tlsConn := tls.Client(conn, h.Config)
	if err := tlsConn.Handshake(); err != nil {
		return nil, err
	}
	return &HandshakeResult{
		tlsConn.ConnectionState().Version, tlsConn.ConnectionState().CipherSuite,
	}, nil
}

// DialerConfig specifies configuration for dialing.
type DialerConfig struct {
	// A Secret pre-shared between listeners and dialers. This value must be set.
	Secret Secret

	// Handshaker allows for customization of the handshake with the origin server. This can be
	// important as the handshake performed by the Go standard library can be fingerprinted. May not
	// be nil.
	Handshaker Handshaker

	// NonceTTL specifies the time-to-live for nonces used in completion signals. DefaultNonceTTL is
	// used if NonceTTL is unspecified.
	NonceTTL time.Duration
}

func (cfg DialerConfig) withDefaults() DialerConfig {
	newCfg := cfg
	if cfg.NonceTTL == 0 {
		newCfg.NonceTTL = DefaultNonceTTL
	}
	return newCfg
}

// Dialer is the interface implemented by network dialers.
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type dialer struct {
	Dialer
	DialerConfig
}

func (d dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d.Handshaker == nil {
		return nil, errors.New("handshaker must not be nil")
	}
	conn, err := d.Dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return Client(conn, d.DialerConfig), nil
}

// WrapDialer wraps the input dialer with a network dialer which will perform the ptlshs protocol.
func WrapDialer(d Dialer, cfg DialerConfig) Dialer {
	return dialer{d, cfg.withDefaults()}
}

// Dial a ptlshs listener.
func Dial(network, address string, cfg DialerConfig) (net.Conn, error) {
	return WrapDialer(&net.Dialer{}, cfg).Dial(network, address)
}

// DialTimeout acts like Dial but takes a timeout.
func DialTimeout(network, address string, cfg DialerConfig, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return WrapDialer(&net.Dialer{}, cfg).DialContext(ctx, network, address)
}

// ListenerConfig specifies configuration for listening.
type ListenerConfig struct {
	// DialOrigin is used to create TCP connections to the origin server. Must not be nil.
	DialOrigin func(context.Context) (net.Conn, error)

	// A Secret pre-shared between listeners and dialers.
	Secret Secret

	// NonceSweepInterval determines how often the nonce cache is swept for expired entries. If not
	// specified, DefaultNonceSweepInterval will be used.
	NonceSweepInterval time.Duration

	// NonFatalErrors will be used to log non-fatal errors. These will likely be due to probes.
	NonFatalErrors chan<- error
}

func (cfg ListenerConfig) withDefaults() ListenerConfig {
	newCfg := cfg
	if cfg.NonceSweepInterval == 0 {
		newCfg.NonceSweepInterval = DefaultNonceSweepInterval
	}
	if cfg.NonFatalErrors == nil {
		// Errors are dropped if the channel is full, so this should be fine.
		newCfg.NonFatalErrors = make(chan error)
	}
	return newCfg
}

type listener struct {
	net.Listener
	ListenerConfig
	nonceCache *nonceCache
}

func (l listener) Accept() (net.Conn, error) {
	clientConn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return serverConnWithCache(clientConn, l.ListenerConfig, l.nonceCache, false), nil
}

func (l listener) Close() error {
	l.nonceCache.close()
	return l.Listener.Close()
}

// WrapListener wraps the input listener with one which speaks the ptlshs protocol.
func WrapListener(l net.Listener, cfg ListenerConfig) net.Listener {
	cfg = cfg.withDefaults()
	return listener{l, cfg, newNonceCache(cfg.NonceSweepInterval)}
}

// Listen for ptlshs dialers.
func Listen(network, address string, cfg ListenerConfig) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return WrapListener(l, cfg), nil
}
