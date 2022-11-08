// tinywss
//
// A module for establishing a rudimentary secure "websocket".
// Performs websocket handshake, but does not actually
// enforce the websocket protocol for data exchanged afterwards.
// Exposes a dialer and listener returning objects conforming to
// net.Conn and net.Listener.
//
// It is not meant to be compatible with anything but itself.
//
package tinywss

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/getlantern/golog"
)

var (
	// ErrListenerClosed is the error returned if listener is used after closed
	ErrListenerClosed = errors.New("listener closed")
	// ErrDialerClosed is the error returned if client is used after closed
	ErrClientClosed         = errors.New("client closed")
	log                     = golog.LoggerFor("tinywss")
	dialSessionTimeout      = 1 * time.Minute
	defaultHandshakeTimeout = 10 * time.Second
	defaultProtocols        = []string{ProtocolMux, ProtocolRaw}
)

const (
	// ProtocolRaw specifies a raw (not multiplexed) connection
	ProtocolRaw = "tinywss-raw"
	// ProtocolMux specifies a multiplexed connection
	ProtocolMux            = "tinywss-smux"
	defaultMaxPendingDials = 1024
)

// HandshakeError is returned when handshake expectations fail
type HandshakeError struct {
	message string
}

func (e HandshakeError) Error() string { return e.message }

func handshakeErr(message string) error {
	return HandshakeError{
		message: "websocket handshake: " + message,
	}
}

type WsConn struct {
	net.Conn
	protocol string
	onClose  func()
	headers  http.Header
}

// Wrapped implements the interface netx.WrappedConn
func (c *WsConn) Wrapped() net.Conn {
	return c.Conn
}

// implements net.Conn.Close()
func (c *WsConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return c.Conn.Close()
}

// returns the headers on the initial HTTP connection that was upgraded
// to create this WsConn.
func (c *WsConn) UpgradeHeaders() http.Header {
	return c.headers
}

type Client interface {

	// DialContext attempts to dial the configured server, returning an error if
	// the context given expires before the server can be contacted.
	DialContext(ctx context.Context) (net.Conn, error)

	// Close shuts down any resources associated with the client
	Close() error
}

// RoundTripHijacker is the interface used by the Client to make the
// HTTP upgrade request and hijack the the underlying connection.
type RoundTripHijacker interface {
	RoundTripHijack(*http.Request) (*http.Response, net.Conn, error)
}
