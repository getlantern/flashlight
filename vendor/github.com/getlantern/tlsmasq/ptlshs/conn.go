package ptlshs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/getlantern/preconn"
	"github.com/getlantern/tlsutil"
)

// Conn is a network connection between two peers speaking the ptlshs protocol. Methods returning
// connection state data (TLSVersion, CipherSuite, etc.) block until the handshake is complete.
//
// Connections returned by listeners and dialers in this package will implement this interface.
// However, most users of this package can ignore this type.
type Conn interface {
	net.Conn

	// Handshake executes the ptlshs handshake protocol, if it has not yet been performed. Note
	// that, per the protocol, the connection will proxy all data until the completion signal. Thus,
	// if this connection comes from an active probe, this handshake function may not return until
	// the probe closes the connection on its end. As a result, this function should be treated as
	// one which may be long-running or never return.
	Handshake() error

	// TLSVersion is the TLS version negotiated during the proxied handshake.
	TLSVersion() uint16

	// CipherSuite is the cipher suite negotiated during the proxied handshake.
	CipherSuite() uint16

	// NextSeq increments and returns the connection's sequence number. The starting sequence number
	// is derived from the server random in the proxied handshake. Clients and servers will have the
	// same derived sequence numbers, so this can be used in cipher suites which use the sequence
	// number as a nonce.
	NextSeq() [8]byte

	// IV is an initialization vector. This is derived from the server random in the proxied handshake.
	// Dialers and listeners will have the same IV, so this can be used when needed in ciphers.
	IV() [16]byte
}

type connState struct {
	version, suite uint16

	seq [8]byte
	iv  [16]byte

	seqLock sync.Mutex
}

func (s *connState) nextSeq() [8]byte {
	s.seqLock.Lock()
	defer s.seqLock.Unlock()

	// Taken from crypto/tls.halfConn.incSeq.
	for i := 7; i >= 0; i-- {
		s.seq[i]++
		if s.seq[i] != 0 {
			return s.seq
		}
	}

	// Not allowed to let sequence number wrap.
	// Instead, must renegotiate before it does.
	// Not likely enough to bother.
	panic("ptlshs: sequence number wraparound")
}

type clientConn struct {
	// The underlying connection to the server. This is likely just a TCP connection.
	net.Conn

	cfg DialerConfig

	// One of the following is initialized after Handshake().
	state        *connState
	handshakeErr error

	shakeOnce, closeOnce *once
}

// Client initializes a client-side connection.
func Client(toServer net.Conn, cfg DialerConfig) Conn {
	cfg = cfg.withDefaults()
	return &clientConn{toServer, cfg, nil, nil, newOnce(), newOnce()}
}

func (c *clientConn) Read(b []byte) (n int, err error) {
	if err := c.Handshake(); err != nil {
		return 0, wrapError("handshake failed", err)
	}
	return c.Conn.Read(b)
}

func (c *clientConn) Write(b []byte) (n int, err error) {
	if err := c.Handshake(); err != nil {
		return 0, wrapError("handshake failed", err)
	}
	return c.Conn.Write(b)
}

// Handshake performs the ptlshs handshake protocol, if it has not yet been performed. Note that,
// per the protocol, the connection will proxy all data until the completion signal. Thus, if this
// connection comes from an active probe, this handshake function may not return until the probe
// closes the connection on its end. As a result, this function should be treated as one which may
// be long-running or never return.
//
// May return tlsutil.UnexpectedAlertError.
func (c *clientConn) Handshake() error {
	return c.shakeOnce.do(func() error {
		handshakeErr := c.handshake()
		if c.closeOnce.isDone() {
			// c.handshake will exit early if the connection is closed, but sometimes with a
			// seemingly unrelated error. It is easiest to just translate the error here.
			return net.ErrClosed
		}
		return handshakeErr
	})
}

func (c *clientConn) handshake() error {
	var (
		transcriptHMAC = signalHMAC(c.cfg.Secret)
		transcriptDone = false
		serverRandom   []byte
	)
	onClientRead := func(b []byte) error {
		if !transcriptDone {
			transcriptHMAC.Write(b)
		}
		if serverRandom != nil {
			return nil
		}
		serverHello, err := tlsutil.ParseServerHello(b)
		if err != nil {
			return fmt.Errorf("failed to parse ServerHello: %w", err)
		}
		serverRandom = serverHello.Random
		return nil
	}
	defer func() { transcriptDone = true }()

	originalConn := c.Conn
	c.Conn = mitm(c.Conn, onClientRead, nil)
	defer func() { c.Conn = originalConn }()

	hsResult, err := c.cfg.Handshaker.Handshake(c.Conn)
	if err != nil {
		return err
	}
	if serverRandom == nil {
		return errors.New("never saw ServerHello")
	}
	seq, iv, err := deriveSeqAndIV(serverRandom)
	if err != nil {
		return fmt.Errorf("failed to derive sequence and IV: %w", err)
	}
	tlsState, err := tlsutil.NewConnectionState(
		hsResult.Version, hsResult.CipherSuite, c.cfg.Secret, iv, seq)
	if err != nil {
		return fmt.Errorf("failed to read TLS connection state: %w", err)
	}
	signal, err := newClientSignal(c.cfg.NonceTTL)
	if err != nil {
		return fmt.Errorf("failed to create completion signal: %w", err)
	}
	if _, err = tlsutil.WriteRecord(c.Conn, *signal, tlsState); err != nil {
		return wrapError("failed to signal completion", err)
	}
	// The watchForCompletionFunction needs direct control over what is written to the transcript.
	transcriptDone = true
	if err := c.watchForCompletion(tlsState, transcriptHMAC); err != nil {
		return wrapError("error watching for server completion signal", err)
	}
	// We're overwriting concurrently accessed fields here. However, these are not used concurrently
	// until the handshake is complete.
	c.state = &connState{hsResult.Version, hsResult.CipherSuite, seq, iv, sync.Mutex{}}
	return nil
}

// Other than its inclusion in the transcript, any data read off c.Conn before the signal is
// discarded. This data is post-handshake data sent by the origin and forwarded by the tlsmasq
// server. Passing this data on in calls to c.Read would disrupt the next phase of the connection.
//
// transcriptHMAC should reflect everything already received on c.Conn. This will be used to verify
// the transcript, using a MAC contained in the server signal. This prevents an attack in which a
// bad actor could inject garbage data, see that the connection is unaffected, and conclude that it
// is a tlsmasq connection.
//
// See https://github.com/getlantern/lantern-internal/issues/4507
func (c *clientConn) watchForCompletion(tlsState *tlsutil.ConnectionState, transcriptHMAC hash.Hash) error {
	readBuf := new(bytes.Buffer)
	onRead := func(b []byte) error {
		readBuf.Write(b)
		return nil
	}
	conn := mitm(c.Conn, onRead, nil)

	// We attempt to decrypt every record we see from the server. We assume that any records we are
	// unable to decrypt must have come from the origin. The first record we successfully decrypt
	// should be the server signal.

	unprocessedBuf := new(bufferList)
	for {
		r := io.MultiReader(unprocessedBuf, conn)
		recordData, unprocessed, err := tlsutil.ReadRecord(r, tlsState)
		if len(unprocessed) > 0 {
			unprocessedBuf.prepend(unprocessed)
		}
		if err != nil {
			if c.closeOnce.isDone() {
				return err
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return io.ErrUnexpectedEOF
			}
			if !errors.As(err, new(tlsutil.DecryptError)) {
				return err
			}
			// If we failed to decrypt, then this must not have been the signal.
			processed := readBuf.Next(readBuf.Len() - unprocessedBuf.len())
			transcriptHMAC.Write(processed)
			continue
		}

		ss, err := parseServerSignal(recordData)
		if err != nil {
			return fmt.Errorf("decrypted record, but failed to parse as signal: %w", err)
		}
		if !ss.validMAC(transcriptHMAC.Sum(nil)) {
			return fmt.Errorf("server signal contains bad transcript MAC")
		}
		// Put unprocessed post-signal data back on the connection.
		c.Conn = preconn.WrapReader(c.Conn, unprocessedBuf)
		return nil
	}
}

func (c *clientConn) TLSVersion() uint16 {
	c.shakeOnce.wait()
	if c.state == nil {
		return 0
	}
	return c.state.version
}

func (c *clientConn) CipherSuite() uint16 {
	c.shakeOnce.wait()
	if c.state == nil {
		return 0
	}
	return c.state.suite
}

func (c *clientConn) NextSeq() [8]byte {
	c.shakeOnce.wait()
	if c.state == nil {
		return [8]byte{}
	}
	return c.state.nextSeq()
}

func (c *clientConn) IV() [16]byte {
	c.shakeOnce.wait()
	if c.state == nil {
		return [16]byte{}
	}
	return c.state.iv
}

func (c *clientConn) Close() error {
	return c.closeOnce.do(func() error {
		return c.Conn.Close()
	})
}

type serverConn struct {
	// The underlying connection to the client. This is likely just a TCP connection.
	wrapped     net.Conn
	wrappedLock sync.RWMutex

	cfg ListenerConfig

	nonceCache *nonceCache
	closeCache bool

	// Initialized if the handshake completes successfully.
	state *connState

	shakeOnce, closeOnce *once

	// Only used prior to and during the handshake.
	hsReadDeadline, hsWriteDeadline deadline
	deadlinesLock                   sync.Mutex
}

// Server initializes a server-side connection.
func Server(toClient net.Conn, cfg ListenerConfig) Conn {
	cfg = cfg.withDefaults()
	nc := newNonceCache(cfg.NonceSweepInterval)
	return serverConnWithCache(toClient, cfg, nc, true)
}

// Ignores cfg.NonceSweepInterval.
func serverConnWithCache(toClient net.Conn, cfg ListenerConfig, cache *nonceCache, closeCache bool) Conn {
	return &serverConn{
		toClient, sync.RWMutex{}, cfg, cache, closeCache, nil,
		newOnce(), newOnce(), newDeadline(), newDeadline(), sync.Mutex{}}
}

func (c *serverConn) getWrapped() net.Conn {
	c.wrappedLock.RLock()
	defer c.wrappedLock.RUnlock()
	return c.wrapped
}

func (c *serverConn) setWrapped(conn net.Conn) {
	c.wrappedLock.Lock()
	c.wrapped = conn
	c.wrappedLock.Unlock()
}

func (c *serverConn) Read(b []byte) (n int, err error) {
	handshakeErr := make(chan error, 1)
	go func() { handshakeErr <- c.Handshake() }()
	select {
	case err := <-handshakeErr:
		if err != nil {
			return 0, fmt.Errorf("handshake failed: %w", err)
		}
		return c.getWrapped().Read(b)
	case <-c.hsReadDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *serverConn) Write(b []byte) (n int, err error) {
	handshakeErr := make(chan error, 1)
	go func() { handshakeErr <- c.Handshake() }()
	select {
	case err := <-handshakeErr:
		if err != nil {
			return 0, fmt.Errorf("handshake failed: %w", err)
		}
		return c.getWrapped().Write(b)
	case <-c.hsWriteDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *serverConn) Close() error {
	return c.closeOnce.do(func() error {
		if c.closeCache {
			c.nonceCache.close()
		}
		return c.getWrapped().Close()
	})
}

func (c *serverConn) SetDeadline(t time.Time) error {
	c.deadlinesLock.Lock()
	defer c.deadlinesLock.Unlock()

	// n.b. If one deadline is closed, so is the other.
	if !c.hsReadDeadline.isClosed() {
		c.hsReadDeadline.set(t)
		c.hsWriteDeadline.set(t)
		return nil
	}
	return c.getWrapped().SetDeadline(t)
}

func (c *serverConn) SetReadDeadline(t time.Time) error {
	c.deadlinesLock.Lock()
	defer c.deadlinesLock.Unlock()

	if !c.hsReadDeadline.isClosed() {
		c.hsReadDeadline.set(t)
		return nil
	}
	return c.getWrapped().SetReadDeadline(t)
}

func (c *serverConn) SetWriteDeadline(t time.Time) error {
	c.deadlinesLock.Lock()
	defer c.deadlinesLock.Unlock()

	if !c.hsWriteDeadline.isClosed() {
		c.hsWriteDeadline.set(t)
		return nil
	}
	return c.getWrapped().SetWriteDeadline(t)
}

func (c *serverConn) LocalAddr() net.Addr {
	return c.getWrapped().LocalAddr()
}

func (c *serverConn) RemoteAddr() net.Addr {
	return c.getWrapped().RemoteAddr()
}

// Handshake performs the ptlshs handshake protocol, if it has not yet been performed. Note that,
// per the protocol, the connection will proxy all data until the completion signal. Thus, if this
// connection comes from an active probe, this handshake function may not return until the probe
// closes the connection on its end. As a result, this function should be treated as one which may
// be long-running or never return.
//
// May return tlsutil.UnexpectedAlertError.
func (c *serverConn) Handshake() error {
	return c.shakeOnce.do(func() error {
		handshakeErr := c.handshake()
		if c.closeOnce.isDone() {
			// c.handshake will exit early if the connection is closed, but sometimes with a
			// seemingly unrelated error. It is easiest to just translate the error here.
			return net.ErrClosed
		}
		return handshakeErr
	})
}

func (c *serverConn) handshake() error {
	// Use a context to ensure this function exits if c is closed.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-c.closeOnce.done:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	origin, err := c.cfg.DialOrigin(ctx)
	if err != nil {
		return fmt.Errorf("failed to dial origin server: %w", err)
	}
	defer origin.Close()

	transcriptHMAC := signalHMAC(c.cfg.Secret)
	transcriptLock := new(sync.Mutex)
	originalWrapped := c.getWrapped()
	onClientWrite := func(b []byte) error {
		transcriptLock.Lock()
		transcriptHMAC.Write(b)
		transcriptLock.Unlock()
		return nil
	}
	c.setWrapped(mitm(c.getWrapped(), nil, onClientWrite))
	stopTranscript := func() { c.setWrapped(originalWrapped) }
	defer stopTranscript() // may end up a no-op, but that's okay

	// Read and copy ClientHello.
	b, err := readClientHello(ctx, c.getWrapped(), listenerReadBufferSize)
	if err != nil && !errors.As(err, new(networkError)) {
		// Client sent something other than ClientHello. Proxy everything to match origin behavior.
		stopTranscript()
		proxyUntilClose(ctx, preconn.Wrap(c.getWrapped(), b), origin)
		return fmt.Errorf("did not receive ClientHello: %w", err)
	}
	if err != nil {
		return fmt.Errorf("failed to read ClientHello: %w", err)
	}
	_, err = origin.Write(b)
	if err != nil {
		return fmt.Errorf("failed to write to origin: %w", err)
	}

	// Read, parse, and copy ServerHello.
	b, c.state, err = readServerHello(ctx, origin, listenerReadBufferSize)
	if err != nil && !errors.As(err, new(networkError)) {
		// Origin sent something other than ServerHello. Proxy everything to match origin behavior.
		stopTranscript()
		proxyUntilClose(ctx, c.getWrapped(), preconn.Wrap(origin, b))
		return fmt.Errorf("did not receive ServerHello: %w", err)
	}
	if err != nil {
		return fmt.Errorf("failed to parse ServerHello: %w", err)
	}
	tlsState, err := tlsutil.NewConnectionState(
		c.state.version, c.state.suite, c.cfg.Secret, c.state.iv, c.state.seq)
	if err != nil {
		return fmt.Errorf("failed to init conn state based on hello info: %w", err)
	}
	_, err = c.getWrapped().Write(b)
	if err != nil {
		return fmt.Errorf("failed to write to client: %w", err)
	}

	// Wait until we've received the client's completion signal.
	if err := c.watchForCompletion(ctx, listenerReadBufferSize, tlsState, origin); err != nil {
		return fmt.Errorf("failed while watching for completion signal: %w", err)
	}

	// Send our own completion signal.
	transcriptLock.Lock()
	signal, err := newServerSignal(transcriptHMAC.Sum(nil))
	transcriptLock.Unlock()
	if err != nil {
		return fmt.Errorf("failed to create completion signal: %w", err)
	}

	_, err = tlsutil.WriteRecord(c.getWrapped(), *signal, tlsState)
	if err != nil {
		return fmt.Errorf("failed to signal completion: %w", err)
	}

	// Transfer handshake deadlines to the underlying connection.
	c.deadlinesLock.Lock()
	c.getWrapped().SetReadDeadline(c.hsReadDeadline.get())
	c.getWrapped().SetWriteDeadline(c.hsWriteDeadline.get())
	c.hsReadDeadline.close()
	c.hsWriteDeadline.close()
	c.deadlinesLock.Unlock()

	return nil
}

// Copies data between the client (c.Conn) and the origin server, watching client messages for the
// completion signal. If the signal is received, the origin connection will be closed. If an error
// is returned, then either the origin or the client connection was broken.
func (c *serverConn) watchForCompletion(ctx context.Context, bufferSize int,
	tlsState *tlsutil.ConnectionState, originConn net.Conn) error {

	// We will set up a bi-directional copy between the client and the origin, watching everything
	// sent by the client. We attempt to decrypt every record we see from the client using the input
	// state. If we see the signal, we close the connection with the origin. Otherwise, we continue
	// to proxy.

	toClient, toOrigin := newCancelConn(c.getWrapped()), newCancelConn(originConn)

	nonFatalError := func(err error) {
		select {
		case c.cfg.NonFatalErrors <- err:
		default:
		}
	}

	isSignal := func(record []byte) bool {
		data, _, err := tlsutil.ReadRecord(bytes.NewReader(record), tlsState)
		if err != nil {
			// We expect that we will fail to decrypt a few records before we see the signal. Thus
			// we do not log this as an error.
			return false
		}
		signal, err := parseClientSignal(data)
		if err != nil {
			nonFatalError(fmt.Errorf("decrypted record, but failed to parse signal: %w", err))
			return false
		}
		if !c.nonceCache.isValid(signal.getNonce()) {
			nonFatalError(errors.New("received bad nonce; likely a signal replay"))
			return false
		}
		return true
	}

	foundSignal := make(chan struct{})
	signalFound := func(preSignal, postSignal []byte) {
		close(foundSignal)

		// We stop both copy routines by closing the connection to the origin server. We first
		// attempt to flush any unprocessed data.
		toOrigin.Write(preSignal)
		toOrigin.Close()

		// We also need to ensure the unprocessed post-signal data is not lost. We prepend it to
		// the client connection. Access to c.Conn is single-threaded until the handshake is
		// complete, so this is safe to do without synchronization.
		c.setWrapped(preconn.Wrap(c.getWrapped(), postSignal))
	}

	rr := new(recordReader)
	onClientRead := func(b []byte) error {
		// There may be a partial record buffered in rr.
		currentRecordStart := -1 * rr.currentlyBuffered()
		for _, record := range rr.read(b) {
			if isSignal(record) {
				// The signal may have started before b, so we adjust to 0 if necessary.
				signalStart := max(0, currentRecordStart)
				signalEnd := currentRecordStart + len(record)
				signalFound(b[:signalStart], b[signalEnd:])
				return nil
			}
			currentRecordStart += len(record)
		}
		return nil
	}

	var (
		client     = mitm(toClient, onClientRead, nil)
		origin     = toOrigin
		copyErr    = make(chan error, 2)
		buf1, buf2 = make([]byte, bufferSize), make([]byte, bufferSize)
	)
	go func() { _, err := io.CopyBuffer(client, origin, buf1); copyErr <- err }()
	go func() { _, err := io.CopyBuffer(origin, client, buf2); copyErr <- err }()

	select {
	case err := <-copyErr:
		// Cancel I/O, then wait for a second value to be sent on copyErr to ensure both copy
		// routines have exited.
		toClient.cancelIO()
		toOrigin.cancelIO()
		select {
		case <-copyErr:
		case <-ctx.Done():
			return ctx.Err()
		}
		if isClosedChannel(foundSignal) {
			return nil
		}
		if err != nil {
			return err
		}
		// We did not find the signal, but at least one side hit EOF.
		return io.ErrUnexpectedEOF

	case <-ctx.Done():
		toClient.cancelIO()
		toOrigin.cancelIO()
		return ctx.Err()
	}
}

func (c *serverConn) TLSVersion() uint16 {
	<-c.shakeOnce.done
	if c.state == nil {
		return 0
	}
	return c.state.version
}

func (c *serverConn) CipherSuite() uint16 {
	<-c.shakeOnce.done
	if c.state == nil {
		return 0
	}
	return c.state.suite
}

func (c *serverConn) NextSeq() [8]byte {
	<-c.shakeOnce.done
	if c.state == nil {
		return [8]byte{}
	}
	return c.state.nextSeq()
}

func (c *serverConn) IV() [16]byte {
	<-c.shakeOnce.done
	if c.state == nil {
		return [16]byte{}
	}
	return c.state.iv
}

// Continuously reads off of conn until one of the following:
// 	- The bytes read constitute a valid TLS ClientHello.
//	- The bytes read could not possibly constitute a valid TLS ClientHello.
//	- A non-temporary network error is encountered.
// Whatever was read is always returned.
func readClientHello(ctx context.Context, conn net.Conn, bufferSize int) ([]byte, error) {
	var (
		buf   = make([]byte, bufferSize)
		read  = new(bytes.Buffer)
		errC  = make(chan error, 1)
		_conn = newCancelConn(conn)
	)
	readHello := func() error {
		for {
			n, err := _conn.Read(buf)
			if err != nil {
				return networkError{err}
			}
			// Note: bytes.Buffer.Write does not return errors.
			read.Write(buf[:n])
			_, err = tlsutil.ValidateClientHello(read.Bytes())
			if err == nil {
				return nil
			}
			if !errors.Is(err, io.EOF) {
				return err
			}
		}
	}
	go func() { errC <- readHello() }()
	select {
	case err := <-errC:
		return read.Bytes(), err
	case <-ctx.Done():
		_conn.cancelIO()
		return read.Bytes(), ctx.Err()
	}
}

// Continuously reads off of conn until one of the following:
// 	- The bytes read constitute a valid TLS ServerHello.
//	- The bytes read could not possibly constitute a valid TLS ServerHello.
//	- A non-temporary network error is encountered.
// Whatever was read is always returned. When a valid ServerHello is read, it is parsed and used to
// create a connection state.
func readServerHello(ctx context.Context, conn net.Conn, bufferSize int) ([]byte, *connState, error) {
	type result struct {
		cs  *connState
		err error
	}

	var (
		buf     = make([]byte, bufferSize)
		read    = new(bytes.Buffer)
		resultC = make(chan result, 1)
		_conn   = newCancelConn(conn)
	)
	readHello := func() result {
		for {
			n, err := _conn.Read(buf)
			if err != nil {
				return result{nil, networkError{err}}
			}
			// Note: bytes.Buffer.Write does not return errors.
			read.Write(buf[:n])
			serverHello, err := tlsutil.ParseServerHello(read.Bytes())
			if err == nil {
				version, suite := serverHello.Version, serverHello.Suite
				seq, iv, err := deriveSeqAndIV(serverHello.Random)
				if err != nil {
					return result{nil, fmt.Errorf("failed to derive sequence and IV: %w", err)}
				}
				return result{&connState{version, suite, seq, iv, sync.Mutex{}}, nil}
			}
			if !errors.Is(err, io.EOF) {
				return result{nil, err}
			}
		}
	}
	go func() { resultC <- readHello() }()
	select {
	case r := <-resultC:
		return read.Bytes(), r.cs, r.err
	case <-ctx.Done():
		_conn.cancelIO()
		return read.Bytes(), nil, ctx.Err()
	}
}

func deriveSeqAndIV(serverRandom []byte) (seq [8]byte, iv [16]byte, err error) {
	// https://tools.ietf.org/html/rfc5246#section-6.1
	// https://tools.ietf.org/html/rfc8446#section-4.1.3
	const serverRandomSize = 32

	if len(serverRandom) != serverRandomSize {
		return seq, iv, fmt.Errorf(
			"expected larger server random (should be 32 bytes, got %d)", len(serverRandom))
	}
	copy(seq[:], serverRandom)
	copy(iv[:], serverRandom[len(seq):])
	return seq, iv, nil
}

type mitmConn struct {
	net.Conn

	// Any errors returned by onRead or onWrite will result in an error returned by Read or Write
	// respectively. Though these callback errors may not reflect actual read or write errors,
	// treating them as such allows for simpler usage of mitmConn.
	onRead, onWrite func([]byte) error
}

// Sets up a MITM'd connection. Callbacks will be invoked synchronously. Either callback may be nil.
func mitm(conn net.Conn, onRead, onWrite func([]byte) error) mitmConn {
	if onRead == nil {
		onRead = func(_ []byte) error { return nil }
	}
	if onWrite == nil {
		onWrite = func(_ []byte) error { return nil }
	}
	return mitmConn{conn, onRead, onWrite}
}

func (c mitmConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 {
		if err := c.onRead(b[:n]); err != nil {
			return n, err
		}
	}
	return
}

func (c mitmConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if n > 0 {
		if err := c.onWrite(b[:n]); err != nil {
			return n, err
		}
	}
	return
}

// Different than a net.Error. Used only to distinguish network errors from protocol-level errors.
type networkError struct {
	cause error
}

func (err networkError) Error() string {
	return err.cause.Error()
}

func (err networkError) Unwrap() error {
	return err.cause
}

// Used to ensure we pass back net.Errors when wrapped connections return them to us.
type wrappedNetError struct {
	wrapped net.Error
	msg     string
}

func (err wrappedNetError) Error() string   { return fmt.Sprintf("%s: %v", err.msg, err.wrapped) }
func (err wrappedNetError) Timeout() bool   { return err.wrapped.Timeout() }
func (err wrappedNetError) Temporary() bool { return err.wrapped.Temporary() }
func (err wrappedNetError) Unwrap() error   { return err.wrapped }

// Wraps the input error with the message. Uses wrappedNetError if err is a net.Error. Otherwise,
// fmt.Errorf is used.
func wrapError(msg string, err error) error {
	if netErr, ok := err.(net.Error); ok {
		return wrappedNetError{netErr, msg}
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Proxies until an error is returned on either connection or the context completes.
func proxyUntilClose(ctx context.Context, a, b net.Conn) {
	_a, _b := newCancelConn(a), newCancelConn(b)
	copyDone := make(chan struct{}, 2)
	go func() { io.Copy(_a, _b); copyDone <- struct{}{} }()
	go func() { io.Copy(_b, _a); copyDone <- struct{}{} }()
	select {
	case <-copyDone:
	case <-ctx.Done():
	}
	_a.cancelIO()
	_b.cancelIO()
}

type once struct {
	once sync.Once

	err  error
	done chan struct{}
}

func newOnce() *once {
	return &once{done: make(chan struct{})}
}

func (o *once) wait() {
	<-o.done
}

func (o *once) isDone() bool {
	select {
	case <-o.done:
		return true
	default:
		return false
	}
}

func (o *once) do(f func() error) error {
	o.once.Do(func() {
		o.err = f()
		close(o.done)
	})
	return o.err
}

// Assumes no value is ever sent on c.
func isClosedChannel(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
