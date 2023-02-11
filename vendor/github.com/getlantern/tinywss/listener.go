package tinywss

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/ops"
)

// Configuration options for ListenAddr
type ListenOpts struct {
	Addr             string
	CertFile         string
	KeyFile          string
	TLSConf          *tls.Config
	HandshakeTimeout time.Duration
	Protocols        []string // allowed protocols

	// If provided, this listener is used instead of starting
	// a TLS listener using the tls configuration specified.
	// Addr, CertFile, KeyFile and TLSConf are ignored if this
	// is given.
	Listener net.Listener

	// Multiplex options
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	MaxFrameSize      int
	MaxReceiveBuffer  int
}

// ListenAddr starts a tinywss server listening at the
// configured address.
func ListenAddr(opts *ListenOpts) (net.Listener, error) {
	l, err := listenAddr(opts)
	if err != nil {
		return nil, err
	}

	if l.supportsProtocol(ProtocolMux) {
		return wrapListenerSmux(l, opts)
	} else {
		return l, nil
	}
}

func listenAddr(opts *ListenOpts) (*listener, error) {
	handshakeTimeout := opts.HandshakeTimeout
	if handshakeTimeout == 0 {
		handshakeTimeout = defaultHandshakeTimeout
	}

	var err error
	ll := opts.Listener
	if ll == nil {
		ll, err = newTLSListener(opts)
		if err != nil {
			return nil, err
		}
	} else if opts.Addr != "" {
		return nil, fmt.Errorf("tinywss: cannot specify address and wrapped listener")
	}

	l := &listener{
		connections:      make(chan net.Conn, 1000),
		closed:           make(chan struct{}),
		handshakeTimeout: handshakeTimeout,
		innerListener:    ll,
	}

	var protos []string
	if len(opts.Protocols) > 0 {
		protos = opts.Protocols
	} else {
		protos = defaultProtocols
	}
	l.protocols = make([]string, len(protos))
	copy(l.protocols, protos)

	l.srv = &http.Server{
		Addr:    opts.Addr,
		Handler: http.HandlerFunc(l.handleRequest),
	}

	ops.Go(func() {
		l.listen()
	})
	return l, nil
}

var _ net.Listener = &listener{}

type listener struct {
	srv              *http.Server
	connections      chan net.Conn
	closed           chan struct{}
	mx               sync.Mutex
	innerListener    net.Listener
	protocols        []string
	handshakeTimeout time.Duration
}

func (l *listener) listen() {
	err := l.srv.Serve(l.innerListener)
	if err != http.ErrServerClosed {
		log.Errorf("tinywss listener: %s", err)
	}
	l.Close()
}

// implements net.Listener.Accept
func (l *listener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.connections:
		if !ok {
			return nil, ErrListenerClosed
		}
		return conn, nil
	case <-l.closed:
		return nil, ErrListenerClosed
	}
}

// implements net.Listener.Close
func (l *listener) Close() error {
	l.mx.Lock()
	defer l.mx.Unlock()
	select {
	case <-l.closed:
		return nil
	default:
		close(l.closed)
		return l.srv.Close()
	}
}

// implements net.Listener.Addr
func (l *listener) Addr() net.Addr {
	return l.innerListener.Addr()
}

func (l *listener) handleRequest(w http.ResponseWriter, r *http.Request) {
	conn, err := l.upgrade(w, r)
	if err != nil {
		if conn != nil {
			conn.Close()
		}
		if _, ok := err.(HandshakeError); ok {
			log.Debugf("error upgrading request: %s", err)
		} else {
			log.Errorf("error upgrading request: %s", err)
		}
		return
	}
	l.connections <- conn
}

func (l *listener) upgrade(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if r.Method != "GET" {
		sendError(w, http.StatusBadRequest)
		return nil, handshakeErr("request method must be GET")
	}

	if !headerHasValue(r.Header, "Connection", "upgrade") {
		sendError(w, http.StatusBadRequest)
		return nil, handshakeErr("`Connection` header is missing or invalid")
	}

	if !headerHasValue(r.Header, "Upgrade", "websocket") {
		sendError(w, http.StatusBadRequest)
		return nil, handshakeErr("`Upgrade` header is missing or invalid")
	}

	wskey := r.Header.Get("Sec-Websocket-Key")
	if wskey == "" {
		sendError(w, http.StatusBadRequest)
		return nil, handshakeErr("`Sec-WebSocket-Key' header is missing or invalid")
	}

	wsproto := r.Header.Get("Sec-Websocket-Protocol")
	if !l.supportsProtocol(wsproto) {
		sendError(w, http.StatusBadRequest)
		return nil, handshakeErr(fmt.Sprintf("`Sec-WebSocket-Protocol' header is missing or invalid (%s)", wsproto))
	}

	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response was not http.Hijacker")
	}
	conn, buf, err := h.Hijack()

	if err != nil {
		return nil, err
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		return conn, err
	}
	if buf.Reader.Buffered() > 0 {
		return conn, handshakeErr("request payload before handshake")
	}

	hdr := make(http.Header, 0)
	hdr.Set("Connection", "Upgrade")
	hdr.Set("Upgrade", "websocket")
	hdr.Set("Sec-WebSocket-Accept", acceptForKey(wskey))
	hdr.Set("Sec-WebSocket-Protocol", wsproto)

	res := bytes.NewBufferString("HTTP/1.1 101 Switching Protocols\r\n")
	hdr.Write(res)
	res.WriteString("\r\n")

	if l.handshakeTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(l.handshakeTimeout))
	}
	if _, err = conn.Write(res.Bytes()); err != nil {
		return conn, err
	}
	if l.handshakeTimeout > 0 {
		conn.SetWriteDeadline(time.Time{})
	}

	return &WsConn{
		Conn:     conn,
		protocol: wsproto,
		headers:  cloneHeaders(r.Header),
	}, nil
}

func (l *listener) supportsProtocol(p string) bool {
	for _, proto := range l.protocols {
		if strings.EqualFold(proto, p) {
			return true
		}
	}
	return false
}

// default inner listener
func newTLSListener(opts *ListenOpts) (net.Listener, error) {
	l, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return nil, err
	}

	var tlsConf *tls.Config
	if opts.TLSConf == nil {
		tlsConf = &tls.Config{}
	} else {
		tlsConf = opts.TLSConf.Clone()
	}

	if !strSliceContains(tlsConf.NextProtos, "http/1.1") {
		tlsConf.NextProtos = append(tlsConf.NextProtos, "http/1.1")
	}

	hasCert := len(tlsConf.Certificates) > 0 || tlsConf.GetCertificate != nil
	if !hasCert || opts.CertFile != "" || opts.KeyFile != "" {
		var err error
		tlsConf.Certificates = make([]tls.Certificate, 1)
		tlsConf.Certificates[0], err = tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if err != nil {
			return nil, err
		}
	}

	tl := tls.NewListener(l, tlsConf)
	return tl, nil
}
