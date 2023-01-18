package mockconn

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// Dialer is a test dialer that provides a net.Dial and net.DialTimeout
// equivalent backed by an in-memory data structure, and that provides access to
// the received data via the Received() method.
type Dialer interface {
	// Like net.Dial
	Dial(network, addr string) (net.Conn, error)

	// Like net.DialTimeout
	DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error)

	// Like net.DialContext
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)

	// Gets the last dialed address
	LastDialed() string

	// Gets all received data
	Received() []byte

	// Returns true if all dialed connections are closed
	AllClosed() bool
}

// SucceedingDialer constructs a new Dialer that responds with the given canned
// responseData.
func SucceedingDialer(responseData []byte) Dialer {
	var mx sync.RWMutex
	return &dialer{
		responseData: responseData,
		received:     &bytes.Buffer{},
		mx:           &mx,
	}
}

// FailingDialer constructs a new Dialer that fails to dial with the given
// error.
func FailingDialer(dialError error) Dialer {
	var mx sync.RWMutex
	return &dialer{
		dialError: dialError,
		mx:        &mx,
	}
}

// SlowDialer wraps a dialer to add a delay when dialing it.
func SlowDialer(d Dialer, delay time.Duration) Dialer {
	d2, ok := d.(*dialer)
	if !ok {
		return d
	}
	d3 := *d2
	d3.delay += delay
	return &d3
}

type slowReader struct {
	delay time.Duration
	r     io.Reader
}

func (r slowReader) Read(b []byte) (int, error) {
	time.Sleep(r.delay)
	return r.r.Read(b)
}

type slowReaderDialer struct {
	Dialer
	delay time.Duration
}

func (d slowReaderDialer) Dial(network, addr string) (net.Conn, error) {
	conn, err := d.Dialer.Dial(network, addr)
	conn2 := conn.(*Conn)
	if conn2 != nil {
		conn2.responseReader = slowReader{d.delay, conn2.responseReader}
	}
	return conn2, err
}

// SlowResponder wraps a dialer to add a delay when writing response to the
// dialed connection.
func SlowResponder(d Dialer, delay time.Duration) Dialer {
	return slowReaderDialer{d, delay}
}

// AutoClose wraps a dialer to close the connection automatically after writing
// response.
func AutoClose(d Dialer) Dialer {
	d2, ok := d.(*dialer)
	if !ok {
		return d
	}
	d3 := *d2
	d3.autoClose = true
	return &d3
}

type dialer struct {
	dialError    error
	delay        time.Duration
	autoClose    bool
	responseData []byte
	lastDialed   string
	numOpen      int
	received     *bytes.Buffer
	mx           *sync.RWMutex
}

func (d *dialer) Dial(network, addr string) (net.Conn, error) {
	d.mx.Lock()
	d.lastDialed = addr
	d.mx.Unlock()
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	if d.dialError != nil {
		return nil, d.dialError
	}
	d.mx.Lock()
	d.numOpen++
	d.mx.Unlock()
	return &Conn{
		autoClose:      d.autoClose,
		responseReader: bytes.NewBuffer(d.responseData),
		received:       d.received,
		mx:             d.mx,
		onClose: func() {
			d.numOpen--
		},
	}, nil
}

func (d *dialer) DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	return d.Dial(network, addr)
}

func (d *dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.Dial(network, addr)
}

func (d *dialer) LastDialed() string {
	d.mx.RLock()
	defer d.mx.RUnlock()
	return d.lastDialed
}

func (d *dialer) Received() []byte {
	d.mx.RLock()
	defer d.mx.RUnlock()
	return d.received.Bytes()
}

func (d *dialer) AllClosed() bool {
	d.mx.RLock()
	defer d.mx.RUnlock()
	return d.numOpen == 0
}

func New(received *bytes.Buffer, responseReader io.Reader) *Conn {
	return NewConn(received, responseReader, nil, nil)
}

// NewFailingOnRead returns a connection that fails on read using a static
// error.
func NewFailingOnRead(received *bytes.Buffer, responseReader io.Reader, readError error) *Conn {
	return NewConn(received, responseReader, readError, nil)
}

// NewFailingOnWrite returns a connection that fails on write using a static
// error.
func NewFailingOnWrite(received *bytes.Buffer, responseReader io.Reader, writeError error) *Conn {
	return NewConn(received, responseReader, nil, writeError)
}

// NewConn creates a new mock net.Conn.
func NewConn(received *bytes.Buffer, responseReader io.Reader, readError error, writeError error) *Conn {
	var mx sync.RWMutex
	if received == nil {
		received = bytes.NewBuffer(nil)
	}
	if responseReader == nil {
		responseReader = bytes.NewReader([]byte{})
	}
	return &Conn{received: received, responseReader: responseReader, readError: readError, writeError: writeError, mx: &mx}
}

type Conn struct {
	responseReader io.Reader
	autoClose      bool
	received       *bytes.Buffer
	closed         bool
	onClose        func()
	writeError     error
	readError      error
	mx             *sync.RWMutex
}

func (c *Conn) Read(b []byte) (n int, err error) {
	defer func() {
		if c.autoClose {
			c.Close()
		}
	}()
	c.mx.RLock()
	defer c.mx.RUnlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	if c.readError != nil {
		return 0, c.readError
	}
	return c.responseReader.Read(b)
}

func (c *Conn) Write(b []byte) (n int, err error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	if c.writeError != nil {
		return 0, c.writeError
	}
	return c.received.Write(b)
}

func (c *Conn) Close() error {
	c.mx.Lock()
	defer c.mx.Unlock()
	if !c.closed {
		c.closed = true
		if c.onClose != nil {
			c.onClose()
		}
	}
	return nil
}

func (c *Conn) Closed() bool {
	c.mx.RLock()
	defer c.mx.RUnlock()
	return c.closed
}

func (c *Conn) LocalAddr() net.Addr {
	return &net.TCPAddr{net.ParseIP("127.0.0.1"), 1234, ""}
}

func (c *Conn) RemoteAddr() net.Addr {
	return &net.TCPAddr{net.ParseIP("127.0.0.1"), 4321, ""}
}

func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}
