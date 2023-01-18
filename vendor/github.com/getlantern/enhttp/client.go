package enhttp

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/netx"
	"github.com/getlantern/uuid"
)

// NewDialer creates a new dialer that dials out using the enhttp protocol,
// tunneling via the server specified by serverURL. An http.Client must be
// specified to configure the underlying HTTP behavior.
func NewDialer(client *http.Client, serverURL string) func(string, string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		if addr == "" {
			return nil, errors.New("No address when creating enhttp net.Conn")
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		return &conn{
			id:           id.String(),
			origin:       addr,
			client:       client,
			serverURL:    serverURL,
			readDeadline: intFromTime(time.Now().Add(10 * 365 * 24 * time.Hour)),
			received:     make(chan *result, 10),
			closed:       make(chan struct{}),
			closeErrCh:   make(chan error, 1),
			first:        true,
		}, nil
	}
}

// IsENHTTP indicates whether the given candidate is an ENHTTP client connection
// or a wrapper around one.
func IsENHTTP(candidate net.Conn) bool {
	isClientConn := false
	netx.WalkWrapped(candidate, func(wrapped net.Conn) bool {
		switch wrapped.(type) {
		case *conn:
			isClientConn = true
			return false
		default:
			return true
		}
	})
	return isClientConn
}

type errTimeout string

func (err errTimeout) Error() string {
	return string(err)
}

func (err errTimeout) Timeout() bool {
	return true
}

func (err errTimeout) Temporary() bool {
	return true
}

type result struct {
	b   []byte
	err error
}

type conn struct {
	readDeadline int64
	id           string
	origin       string
	client       *http.Client
	serverURL    string
	received     chan *result
	closed       chan struct{}
	closeErrCh   chan error
	unread       []byte
	first        bool
	closeOnce    sync.Once
	mx           sync.RWMutex
}

func (c *conn) Write(b []byte) (n int, err error) {
	c.mx.RLock()
	serverURL := c.serverURL
	wasFirst := c.first
	c.first = false
	c.mx.RUnlock()

	req, err := http.NewRequest(http.MethodPost, serverURL, bytes.NewReader(b))
	if err != nil {
		return 0, log.Errorf("Error constructing request: %v", err)
	}
	req.Header.Set(ConnectionIDHeader, c.id)
	req.Header.Set(OriginHeader, c.origin)
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, errors.New("Error posting data: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("Unexpected response status posting data: %d", resp.StatusCode)
	}
	updatedServerURL := resp.Header.Get(ServerURL)
	if updatedServerURL != "" {
		c.mx.Lock()
		c.serverURL = updatedServerURL
		c.mx.Unlock()
	}
	if wasFirst {
		go c.receive(resp)
	} else {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
	return len(b), nil
}

func (c *conn) receive(resp *http.Response) {
	defer resp.Body.Close()
	defer close(c.received)

	received := make(chan *result)
	go func() {
		for {
			b := make([]byte, 8192)
			n, err := resp.Body.Read(b)
			// For some reason, when domain-fronting, we often get ErrUnexpectedEOF
			// even though the data transfer seems to work correctly, so for now, just
			// treat it like EOF.
			if err == io.ErrUnexpectedEOF {
				err = io.EOF
			}
			select {
			case <-c.closed:
				return
			case received <- &result{b[:n], err}:
				if err != nil {
					if err != io.EOF {
						log.Debugf("Error on receive: %v", err)
					}
					return
				}
			}
		}
	}()

	for {
		select {
		case <-c.closed:
			return
		case r := <-received:
			c.received <- r
			if r.err != nil {
				return
			}
		}
	}
}

func (c *conn) Read(b []byte) (int, error) {
	if len(c.unread) > 0 {
		return c.readFromUnread(b)
	}

	deadline := timeFromInt(atomic.LoadInt64(&c.readDeadline))
	timeout := deadline.Sub(time.Now())
	select {
	case result, open := <-c.received:
		if !open {
			return 0, io.EOF
		}
		if result.err != nil {
			return 0, result.err
		}
		c.unread = result.b
		return c.Read(b)
	case <-time.After(timeout):
		return 0, errTimeout("i/o timeout")
	}
}

func (c *conn) readFromUnread(b []byte) (int, error) {
	copied := copy(b, c.unread)
	c.unread = c.unread[copied:]
	if len(b) <= copied {
		return copied, nil
	}

	// We've consumed unread but have room for more
	select {
	case result, open := <-c.received:
		if !open {
			return copied, io.EOF
		}
		if result.err != nil {
			return copied, result.err
		}
		c.unread = result.b
		n, err := c.Read(b[copied:])
		return copied + n, err
	default:
		// don't block, just return what we have
		return copied, nil
	}
}

func (c *conn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}

func (c *conn) SetReadDeadline(t time.Time) error {
	atomic.StoreInt64(&c.readDeadline, t.UnixNano())
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *conn) LocalAddr() net.Addr {
	return nil
}

func (c *conn) RemoteAddr() net.Addr {
	return nil
}

func (c *conn) Close() error {
	c.closeOnce.Do(func() {
		defer close(c.closeErrCh)

		close(c.closed)
		c.mx.RLock()
		serverURL := c.serverURL
		c.mx.RUnlock()

		req, err := http.NewRequest(http.MethodPost, serverURL, nil)
		if err != nil {
			c.closeErrCh <- errors.New("Error constructing close request: %v", err)
			return
		}
		req.Header.Set(ConnectionIDHeader, c.id)
		req.Header.Set(Close, "true")
		resp, err := c.client.Do(req)
		if err != nil {
			c.closeErrCh <- errors.New("Error posting close request: %v", err)
			return
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			c.closeErrCh <- errors.New("Unexpected response status posting data on close: %d", resp.StatusCode)
			return
		}
	})

	return <-c.closeErrCh
}
