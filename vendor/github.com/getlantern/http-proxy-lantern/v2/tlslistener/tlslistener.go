// Package tlslistener provides a wrapper around tls.Listen that allows
// descending into the wrapped net.Conn
package tlslistener

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"

	"github.com/getlantern/golog"
	"github.com/getlantern/tlsdefaults"

	utls "github.com/refraction-networking/utls"

	"github.com/getlantern/http-proxy-lantern/v2/instrument"
)

// Wrap wraps the specified listener in our default TLS listener.
func Wrap(wrapped net.Listener, keyFile, certFile, sessionTicketKeyFile, firstSessionTicketKey string,
	requireSessionTickets bool, missingTicketReaction HandshakeReaction, allowTLS13 bool,
	instrument instrument.Instrument) (net.Listener, error) {

	cfg, err := tlsdefaults.BuildListenerConfig(wrapped.Addr().String(), keyFile, certFile)
	if err != nil {
		return nil, err
	}

	log := golog.LoggerFor("lantern-proxy-tlslistener")

	utlsConfig := &utls.Config{}
	onKeys := func(keys [][32]byte) {
		utlsConfig.SetSessionTicketKeys(keys)
	}

	// Depending on the ClientHello generated, we use session tickets both for normal
	// session ticket resumption as well as pre-negotiated session tickets as obfuscation.
	// Ideally we'll make this work with TLS 1.3, see:
	// https://github.com/getlantern/lantern-internal/issues/3057
	// https://github.com/getlantern/lantern-internal/issues/3850
	// https://github.com/getlantern/lantern-internal/issues/4111
	if !allowTLS13 {
		cfg.MaxVersion = tls.VersionTLS12
	}

	var firstKey *[32]byte
	if firstSessionTicketKey != "" {
		b, err := base64.StdEncoding.DecodeString(firstSessionTicketKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse session ticket key: %w", err)
		}
		if len(b) != 32 {
			return nil, errors.New("session ticket key should be 32 bytes")
		}
		firstKey = new([32]byte)
		copy(firstKey[:], b)
	}

	expectTickets := sessionTicketKeyFile != ""
	if expectTickets {
		log.Debugf("Will rotate session ticket key and store in %v", sessionTicketKeyFile)
		maintainSessionTicketKey(cfg, sessionTicketKeyFile, firstKey, onKeys)
	}

	listener := &tlslistener{wrapped, cfg, log, expectTickets, requireSessionTickets, utlsConfig, missingTicketReaction, instrument}
	return listener, nil
}

type tlslistener struct {
	wrapped               net.Listener
	cfg                   *tls.Config
	log                   golog.Logger
	expectTickets         bool
	requireTickets        bool
	utlsCfg               *utls.Config
	missingTicketReaction HandshakeReaction
	instrument            instrument.Instrument
}

func (l *tlslistener) Accept() (net.Conn, error) {
	conn, err := l.wrapped.Accept()
	if err != nil {
		return nil, err
	}
	if !l.expectTickets || !l.requireTickets {
		return &tlsconn{tls.Server(conn, l.cfg), conn}, nil
	}
	helloConn, cfg := newClientHelloRecordingConn(conn, l.cfg, l.utlsCfg, l.missingTicketReaction, l.instrument)
	return &tlsconn{tls.Server(helloConn, cfg), conn}, nil
}

func (l *tlslistener) Addr() net.Addr {
	return l.wrapped.Addr()
}

func (l *tlslistener) Close() error {
	return l.wrapped.Close()
}

type tlsconn struct {
	net.Conn
	wrapped net.Conn
}

func (conn *tlsconn) Wrapped() net.Conn {
	return conn.wrapped
}
