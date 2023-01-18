package tlsmasq

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/getlantern/golog"
	"github.com/getlantern/tlsmasq"
	"github.com/getlantern/tlsmasq/ptlshs"
	"github.com/getlantern/tlsutil"
)

var log = golog.LoggerFor("tlsmasq-listener")

func Wrap(ll net.Listener, certFile string, keyFile string, originAddr string, secret string,
	tlsMinVersion uint16, tlsCipherSuites []uint16, onNonFatalErrors func(error)) (net.Listener, error) {

	var secretBytes ptlshs.Secret
	_secretBytes, decodeErr := hex.DecodeString(secret)
	if decodeErr != nil {
		return nil, fmt.Errorf(`unable to decode secret string "%v": %v`, secret, decodeErr)
	}
	if copy(secretBytes[:], _secretBytes) != 52 {
		return nil, fmt.Errorf(`secret string did not parse to 52 bytes: "%v"`, secret)
	}

	cert, keyErr := tls.LoadX509KeyPair(certFile, keyFile)
	if keyErr != nil {
		return nil, fmt.Errorf("unable to load key file for tlsmasq: %v", keyErr)
	}

	dialOrigin := func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", originAddr)
	}

	nonFatalErrChan := make(chan error)
	go func() {
		for err := range nonFatalErrChan {
			onNonFatalErrors(err)
		}
	}()

	listenerCfg := tlsmasq.ListenerConfig{
		ProxiedHandshakeConfig: ptlshs.ListenerConfig{
			DialOrigin: dialOrigin,
			Secret:     secretBytes,
		},
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tlsMinVersion,
			CipherSuites: tlsCipherSuites,
		},
	}

	return wrapListener(ll, listenerCfg), nil
}

type loggingListener struct {
	tlsmasqListener net.Listener
}

func wrapListener(transportListener net.Listener, cfg tlsmasq.ListenerConfig) net.Listener {
	return loggingListener{tlsmasq.WrapListener(transportListener, cfg)}
}

func (l loggingListener) Accept() (net.Conn, error) {
	conn, err := l.tlsmasqListener.Accept()
	if err != nil {
		return nil, err
	}
	return loggingConn{Conn: conn.(tlsmasq.Conn)}, nil
}

func (l loggingListener) Addr() net.Addr { return l.tlsmasqListener.Addr() }
func (l loggingListener) Close() error   { return l.tlsmasqListener.Close() }

type loggingConn struct {
	tlsmasq.Conn
	handshakeOnce sync.Once
}

func (conn loggingConn) Read(b []byte) (n int, err error)  { return conn.doIO(b, conn.Conn.Read) }
func (conn loggingConn) Write(b []byte) (n int, err error) { return conn.doIO(b, conn.Conn.Write) }

func (conn loggingConn) doIO(b []byte, io func([]byte) (int, error)) (n int, err error) {
	conn.handshakeOnce.Do(func() {
		var alertErr tlsutil.UnexpectedAlertError
		if err = conn.Handshake(); err != nil && errors.As(err, &alertErr) {
			log.Debugf("received alert from origin in tlsmasq handshake: %v", alertErr.Alert)
		}
	})
	if err != nil {
		return 0, err
	}
	return io(b)
}
