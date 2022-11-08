package quicproxy

import "errors"

var (
	ErrNetListen              = errors.New("net.Listen")
	ErrServerServe            = errors.New("server.Serve")
	ErrQuicOpenStreamSync     = errors.New("quic.OpenStreamSync")
	ErrQuicDialAddr           = errors.New("quic.DialAddr")
	ErrQuicOpenStream         = errors.New("quic.OpenStream")
	ErrPinnedCertNotFound     = errors.New("pinned cert not found")
	ErrAddrNotFound           = errors.New("addr not found")
	ErrCertOrPrivKeyNotFound  = errors.New("cert or private key not found")
	ErrX509KeyPair            = errors.New("tls.X509KeyPair")
	ErrNewQuicListener        = errors.New("quic.NewListener")
	ErrQuicListenAddr         = errors.New("quic.ListenAddr")
	ErrQuicCloseWithError     = errors.New("quic.session.CloseWithError")
	ErrNoReverseProxyAddr     = errors.New("no reverse proxy addr")
	ErrQuicListenerClosed     = errors.New("quic listener closed")
	ErrQuicAcceptStreamFailed = errors.New("quic.AcceptStream failed")
)
