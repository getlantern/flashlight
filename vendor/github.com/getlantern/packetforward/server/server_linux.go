// server provides the server end of packetforward functionality. The server reads
// IP packets from the client's connection, forwards these to the final origin using
// gonat and writes response packets back to the client.
package server

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/framed"
	"github.com/getlantern/golog"
	"github.com/getlantern/gonat"
	"github.com/getlantern/idletiming"
	"github.com/oxtoacart/bpool"
)

var (
	log = golog.LoggerFor("packetforward")

	// ErrNoConnection means that we attempted to write to client for which we have no current connection
	ErrNoConnection = errors.New("no client connection")
)

const (
	// DefaultBufferPoolSize is 1MB
	DefaultBufferPoolSize = 1000000

	// DefaultReadBufferSize is gonat.MaximumIPPacketSize
	DefaultReadBufferSize = gonat.MaximumIPPacketSize
)

const (
	maxListenDelay = 1 * time.Second

	baseIODelay = 250 * time.Millisecond
	maxIODelay  = 10 * time.Second
)

type server struct {
	successfulReads  int64
	failedReads      int64
	successfulWrites int64
	failedWrites     int64
	opts             *Opts
	clients          map[string]*client
	clientsMx        sync.Mutex
	close            chan interface{}
	closed           chan interface{}
}

// NewServer constructs a new unstarted packetforward Server. The server can be started by
// calling Serve().
func NewServer(opts *Opts) (Server, error) {
	if opts.BufferPoolSize <= 0 {
		opts.BufferPoolSize = DefaultBufferPoolSize
	}

	if opts.ReadBufferSize <= 0 {
		opts.ReadBufferSize = DefaultReadBufferSize
	}

	// Apply defaults
	err := opts.ApplyDefaults()
	if err != nil {
		return nil, err
	}

	opts.BufferPool = framed.NewHeaderPreservingBufferPool(opts.BufferPoolSize, gonat.MaximumIPPacketSize, true)

	s := &server{
		opts:    opts,
		clients: make(map[string]*client),
		close:   make(chan interface{}),
		closed:  make(chan interface{}),
	}
	go s.printStats()
	return s, nil
}

// Serve serves new packetforward client connections inbound on the given Listener.
func (s *server) Serve(l net.Listener) error {
	defer s.Close()
	defer s.forgetClients()

	tempDelay := time.Duration(0)
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				// delay code based on net/http.Server
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if tempDelay > maxListenDelay {
					tempDelay = maxListenDelay
				}
				log.Debugf("Error accepting connection: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return log.Errorf("Error accepting: %v", err)
		}
		tempDelay = 0
		s.handle(conn)
	}
}

func (s *server) handle(conn net.Conn) {
	// use framed protocol
	framedConn := framed.NewReadWriteCloser(conn)
	framedConn.EnableBigFrames()
	framedConn.DisableThreadSafety()
	framedConn.EnableBuffering(s.opts.ReadBufferSize)

	// Read client ID
	b := make([]byte, 36)
	_, err := framedConn.Read(b)
	if err != nil {
		log.Errorf("Unable to read client ID: %v", err)
		return
	}
	id := string(b)

	s.clientsMx.Lock()
	c := s.clients[id]
	if c == nil {
		efc := eventual.NewValue()
		efc.Set(framedConn)
		c = &client{
			id:         id,
			s:          s,
			framedConn: efc,
		}
		c.markActive()

		gn, err := gonat.NewServer(c, &s.opts.Opts)
		if err != nil {
			log.Errorf("Unable to open gonat: %v", err)
			return
		}
		go func() {
			if serveErr := gn.Serve(); serveErr != nil {
				if serveErr != io.EOF {
					log.Errorf("Error handling packets: %v", serveErr)
				}
			}
		}()
		s.clients[id] = c
	} else {
		c.attach(framedConn)
	}
	s.clientsMx.Unlock()
}

func (s *server) forgetClients() {
	s.clientsMx.Lock()
	s.clients = make(map[string]*client)
	s.clientsMx.Unlock()
}

func (s *server) forgetClient(id string) {
	s.clientsMx.Lock()
	delete(s.clients, id)
	s.clientsMx.Unlock()
}

func (s *server) Close() error {
	select {
	case <-s.close:
		// already closed
	default:
		close(s.close)
	}
	<-s.closed
	return nil
}

type client struct {
	failedOnCurrentConn int64
	lastActive          int64
	id                  string
	s                   *server
	framedConn          eventual.Value
	mx                  sync.RWMutex
}

func (c *client) getFramedConn(timeout time.Duration) *framed.ReadWriteCloser {
	_framedConn, ok := c.framedConn.Get(timeout)
	if !ok {
		return nil
	}
	return _framedConn.(*framed.ReadWriteCloser)
}

func (c *client) attach(framedConn io.ReadWriteCloser) {
	oldFramedConn := c.getFramedConn(0)
	if oldFramedConn != nil {
		go oldFramedConn.Close()
	}
	atomic.StoreInt64(&c.failedOnCurrentConn, 0)
	c.framedConn.Set(framedConn)
}

func (c *client) Read(b bpool.ByteSlice) (int, error) {
	i := 0
	for {
		conn := c.getFramedConn(c.s.opts.IdleTimeout)
		if conn == nil || c.idle() {
			return c.finished(io.EOF)
		}

		if c.isFailedOnCurrentConn() {
			// wait for client to reconnect before idling
			i = sleepWithExponentialBackoff(i)
			continue
		}

		// we're not failed, let's read
		i = 0

		n, err := conn.Read(b.Bytes())
		if err == nil {
			c.markActive()
			atomic.AddInt64(&c.s.successfulReads, 1)
			return n, err
		}

		// reading failed, but it might succeed in the future if the client reconnects, so don't give up
		atomic.AddInt64(&c.s.failedReads, 1)
		c.markFailedOnCurrentConn()
	}
}

func (c *client) Write(b bpool.ByteSlice) (int, error) {
	i := 0
	for {
		conn := c.getFramedConn(c.s.opts.IdleTimeout)
		if conn == nil {
			return c.finished(ErrNoConnection)
		}
		if c.idle() {
			return c.finished(idletiming.ErrIdled)
		}

		if c.isFailedOnCurrentConn() {
			// wait for client to reconnect before idling
			i = sleepWithExponentialBackoff(i)
			continue
		}

		// we're not failed, let's write
		i = 0

		n, err := conn.WriteAtomic(b)
		if err == nil {
			atomic.AddInt64(&c.s.successfulWrites, 1)
			c.markActive()
			return n, err
		}

		// writing failed, but it might succeed in the future if the client reconnects, so don't give up
		atomic.AddInt64(&c.s.failedWrites, 1)
		c.markFailedOnCurrentConn()
	}
}

func (c *client) finished(err error) (int, error) {
	current := c.getFramedConn(0)
	if current != nil {
		current.Close()
	}
	c.s.forgetClient(c.id)
	return 0, err
}

func (c *client) markActive() {
	atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
}

func (c *client) markFailedOnCurrentConn() {
	atomic.StoreInt64(&c.failedOnCurrentConn, 1)
	current := c.getFramedConn(0)
	if current != nil {
		current.Close()
	}
}

func (c *client) isFailedOnCurrentConn() bool {
	return atomic.LoadInt64(&c.failedOnCurrentConn) == 1
}

func (c *client) idle() bool {
	return time.Duration(time.Now().UnixNano()-atomic.LoadInt64(&c.lastActive)) > c.s.opts.IdleTimeout
}

func sleepWithExponentialBackoff(i int) int {
	sleepTime := time.Duration(2 << i * baseIODelay)
	if sleepTime > maxIODelay {
		sleepTime = maxIODelay
	} else {
		i++
	}
	time.Sleep(sleepTime)
	return i
}
