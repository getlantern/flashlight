package quicwrapper

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"sync"

	"github.com/getlantern/netx"
	quic "github.com/lucas-clemente/quic-go"
)

// a QuicDialFn is a function that may be used to establish a new QUIC Session
type QuicDialFn func(ctx context.Context, addr string, tlsConf *tls.Config, config *quic.Config) (quic.Connection, error)
type UDPDialFn func(addr string) (net.PacketConn, *net.UDPAddr, error)

var (
	DialWithNetx    QuicDialFn = newDialerWithUDPDialer(DialUDPNetx)
	DialWithoutNetx QuicDialFn = quic.DialAddrContext
	defaultQuicDial QuicDialFn = DialWithNetx
)

type wrappedSession struct {
	quic.Connection
	conn net.PacketConn
}

func (w wrappedSession) CloseWithError(code quic.ApplicationErrorCode, mesg string) error {
	err := w.Connection.CloseWithError(code, mesg)
	err2 := w.conn.Close()
	if err == nil {
		err = err2
	}
	return err
}

// Creates a new QuicDialFn that uses the UDPDialFn given to
// create the underlying net.PacketConn
func newDialerWithUDPDialer(dial UDPDialFn) QuicDialFn {
	return func(ctx context.Context, addr string, tlsConf *tls.Config, config *quic.Config) (quic.Connection, error) {
		udpConn, udpAddr, err := dial(addr)
		if err != nil {
			return nil, err
		}
		ses, err := quic.DialContext(ctx, udpConn, udpAddr, addr, tlsConf, config)
		if err != nil {
			udpConn.Close()
			return nil, err
		}
		return wrappedSession{ses, udpConn}, nil
	}
}

// DialUDPNetx is a UDPDialFn that resolves addresses and obtains
// the net.PacketConn using the netx package.
func DialUDPNetx(addr string) (net.PacketConn, *net.UDPAddr, error) {
	udpAddr, err := netx.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, nil, err
	}
	udpConn, err := netx.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, nil, err
	}
	return udpConn, udpAddr, nil
}

// NewClient returns a client that creates multiplexed
// QUIC connections in a single Session with the given address using
// the provided configuration.
//
// The Session is created using the
// QuicDialFn given, but is not established until
// the first call to Dial(), DialContext() or Connect()
//
// if dial is nil, the default quic dialer is used
func NewClient(addr string, tlsConf *tls.Config, config *Config, dial QuicDialFn) *Client {
	return NewClientWithPinnedCert(addr, tlsConf, config, dial, nil)
}

// NewClientWithPinnedCert returns a new client configured
// as with NewClient, but accepting only a specific given
// certificate.  If the certificate presented by the connected
// server does match the given certificate, the connection is
// rejected. This check is performed regardless of tls.Config
// settings (ie even if InsecureSkipVerify is true)
//
// If a nil certificate is given, the check is not performed and
// any valid certificate according the tls.Config given is accepted
// (equivalent to NewClient behavior)
func NewClientWithPinnedCert(addr string, tlsConf *tls.Config, config *Config, dial QuicDialFn, cert *x509.Certificate) *Client {
	if dial == nil {
		dial = defaultQuicDial
	}

	tlsConf = defaultNextProtos(tlsConf, DefaultClientProtos)

	return &Client{
		session:    nil,
		address:    addr,
		tlsConf:    tlsConf,
		config:     config,
		dial:       dial,
		pinnedCert: cert,
	}

}

type Client struct {
	session    quic.Connection
	muSession  sync.Mutex
	address    string
	tlsConf    *tls.Config
	pinnedCert *x509.Certificate
	config     *Config
	dial       QuicDialFn
}

// DialContext creates a new multiplexed QUIC connection to the
// server configured in the client. The given Context governs
// cancellation / timeout.  If initial handshaking is performed,
// the operation is additionally governed by HandshakeTimeout
// value given in the client Config.
func (c *Client) DialContext(ctx context.Context) (*Conn, error) {
	session, err := c.getOrCreateSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting session: %w", err)
	}
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		if ne, ok := err.(net.Error); ok && !ne.Temporary() {
			// start over again when seeing unrecoverable error.
			c.clearSession(err.Error())
		}
		return nil, fmt.Errorf("establishing stream: %w", err)
	}
	return newConn(stream, session, session, nil), nil
}

// Dial creates a new multiplexed QUIC connection to the
// server configured for the client.
func (c *Client) Dial() (*Conn, error) {
	return c.DialContext(context.Background())
}

// Connect requests immediate handshaking regardless of
// whether any specific Dial has been initiated. It is
// called lazily on the first Dial if not otherwise
// called.
//
// This can serve to pre-establish a multiplexed
// session, but will also initiate idle timeout
// tracking, keepalives etc. Returns any error
// encountered during handshake.
//
// This may safely be called concurrently with Dial.
// The handshake is guaranteed to be completed when the
// call returns to any caller.
func (c *Client) Connect(ctx context.Context) error {
	_, err := c.getOrCreateSession(ctx)
	return err
}

func (c *Client) getOrCreateSession(ctx context.Context) (quic.Connection, error) {
	c.muSession.Lock()
	defer c.muSession.Unlock()
	if c.session == nil {
		session, err := c.dial(ctx, c.address, c.tlsConf, c.config)
		if err != nil {
			return nil, err
		}
		if c.pinnedCert != nil {
			if err = c.verifyPinnedCert(session); err != nil {
				session.CloseWithError(0, "")
				return nil, err
			}
		}
		c.session = session
	}
	return c.session, nil
}

func (c *Client) verifyPinnedCert(session quic.Connection) error {
	certs := session.ConnectionState().TLS.PeerCertificates
	if len(certs) == 0 {
		return fmt.Errorf("Server did not present any certificates!")
	}

	serverCert := certs[0]
	if !serverCert.Equal(c.pinnedCert) {
		received := pem.EncodeToMemory(&pem.Block{
			Type:    "CERTIFICATE",
			Headers: nil,
			Bytes:   serverCert.Raw,
		})

		expected := pem.EncodeToMemory(&pem.Block{
			Type:    "CERTIFICATE",
			Headers: nil,
			Bytes:   c.pinnedCert.Raw,
		})

		return fmt.Errorf("Server's certificate didn't match expected! Server had\n%v\nbut expected:\n%v", received, expected)
	}
	return nil
}

// closes the session established by this client
// (and all multiplexed connections)
func (c *Client) Close() error {
	c.clearSession("client closed")
	return nil
}

func (c *Client) clearSession(reason string) {
	c.muSession.Lock()
	s := c.session
	c.session = nil
	c.muSession.Unlock()
	if s != nil {
		log.Debugf("Closing quic session (%v)", reason)
		s.CloseWithError(0, "")
	}
}
