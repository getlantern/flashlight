// Package tlsmasq implements a server which masquerades as a different TLS server. For example, the
// server may masquerade as a microsoft.com server, depsite not actually being run by Microsoft.
//
// Clients properly configured with the masquerade protocol can connect and speak to the true
// server, but passive observers will see connections which look like connections to microsoft.com.
// Similarly, active probes will find that the server behaves like a microsoft.com server.
package tlsmasq

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/getlantern/tlsmasq/ptlshs"
)

// DialerConfig specifies configuration for dialing.
type DialerConfig struct {
	// ProxiedHandshakeConfig specifies configuration for the proxied handshake.
	ProxiedHandshakeConfig ptlshs.DialerConfig

	// TLSConfig specifies configuration for the hijacked, true TLS connection with the server. This
	// hijacked connection will use whatever combination of cipher suite and version was negotiated
	// during the proxied handshake. Thus it is important to set fields like CipherSuites and
	// MinVersion to ensure that the security parameters of the hijacked connection are acceptable.
	TLSConfig *tls.Config
}

func (cfg DialerConfig) withDefaults() DialerConfig {
	newCfg := cfg
	if cfg.TLSConfig == nil {
		newCfg.TLSConfig = &tls.Config{}
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
	ptlsDialer := ptlshs.WrapDialer(d.Dialer, d.ProxiedHandshakeConfig)
	conn, err := ptlsDialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return newConn(conn.(ptlshs.Conn), d.TLSConfig, true, d.ProxiedHandshakeConfig.Secret), nil
}

// WrapDialer wraps the input dialer with a network dialer which will perform the tlsmasq protocol.
// Dialing will result in TLS connections with peers.
func WrapDialer(d Dialer, cfg DialerConfig) Dialer {
	return dialer{d, cfg.withDefaults()}
}

// Dial a tlsmasq listener. This will result in a TLS connection with the peer.
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
	// ProxiedHandshakeConfig specifies configuration for the proxied handshake.
	ProxiedHandshakeConfig ptlshs.ListenerConfig

	// TLSConfig specifies configuration for hijacked, true TLS connections with the clients. These
	// hijacked connections will use whatever combination of cipher suite and version was negotiated
	// during the proxied handshake. Thus it is important to set fields like CipherSuites and
	// MinVersion to ensure that the security parameters of the hijacked connections are acceptable.
	TLSConfig *tls.Config
}

func (cfg ListenerConfig) withDefaults() ListenerConfig {
	newCfg := cfg
	if cfg.TLSConfig == nil {
		newCfg.TLSConfig = &tls.Config{}
	}
	return newCfg
}

type listener struct {
	net.Listener // a listener created by the ptlshs package
	ListenerConfig
}

func (l listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	// We know the type assertion will succeed because we know l.Listener comes from ptlshs.
	return newConn(conn.(ptlshs.Conn), l.TLSConfig, false, l.ProxiedHandshakeConfig.Secret), nil
}

// WrapListener wraps the input listener with one which speaks the tlsmasq protocol. Accepted
// connections will be TLS connections.
func WrapListener(l net.Listener, cfg ListenerConfig) net.Listener {
	return listener{ptlshs.WrapListener(l, cfg.ProxiedHandshakeConfig), cfg.withDefaults()}
}

// Listen for tlsmasq dialers. Accepted connections will be TLS connections.
func Listen(network, address string, cfg ListenerConfig) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return listener{ptlshs.WrapListener(l, cfg.ProxiedHandshakeConfig), cfg}, nil
}
