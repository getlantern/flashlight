package cmux

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/xtaci/smux"
)

// DialFN is a function that dials like net.DialContext.
type DialFN func(ctx context.Context, network, addr string) (net.Conn, error)

type DialerOpts struct {
	Dial              DialFN
	PoolSize          int
	BufferSize        int
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
}

type connAndSession struct {
	conn    net.Conn
	session *smux.Session
	dialer  *dialer
	addr    string
	idx     int
}

type dialer struct {
	DialerOpts
	currentConnID int
	pool          map[string]map[int]*connAndSession
	mx            sync.Mutex
}

// Dialer creates a DialFN that returns connections that multiplex themselves
// over a single connection obtained from the underlying opts.Dial function.
// It will continue to use that single connection until and unless it encounters
// an error creating a new multiplexed stream, at which point it will dial
// again.
func Dialer(opts *DialerOpts) DialFN {
	if opts.PoolSize < 1 {
		opts.PoolSize = 1
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}
	d := &dialer{
		DialerOpts: *opts,
		pool:       make(map[string]map[int]*connAndSession)}
	return d.Dial
}

func (d *dialer) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mx.Lock()
	defer d.mx.Unlock()

	idx := d.currentConnID % d.PoolSize
	d.currentConnID++

	var cs *connAndSession

	// Create pool if necessary
	conns := d.pool[addr]
	if conns == nil {
		conns = make(map[int]*connAndSession, d.PoolSize)
		d.pool[addr] = conns
	} else {
		cs = conns[idx]
	}

	// Create conn if necessary
	if cs == nil {
		var err error
		cs, err = d.connect(ctx, network, addr, idx)
		if err != nil {
			return nil, err
		}
		conns[idx] = cs
	}

	// Open stream
	stream, err := cs.session.OpenStream()
	if err != nil {
		// Reconnect and try again
		if err != io.EOF && err != io.ErrClosedPipe {
			log.Debugf("Unexpected error, close existing session before reconnecting: %v", err)
			cs.session.Close()
		}
		cs, err := d.connect(ctx, network, addr, idx)
		if err != nil {
			return nil, err
		}
		stream, err = cs.session.OpenStream()
		if err != nil {
			return nil, err
		}
		conns[idx] = cs
	}

	return &cmconn{
		Conn:    stream,
		onClose: cs.closeIfNecessary,
	}, nil
}

func (d *dialer) connect(ctx context.Context, network, addr string, idx int) (*connAndSession, error) {
	conn, err := d.DialerOpts.Dial(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = d.BufferSize
	if d.KeepAliveInterval > 0 {
		smuxConfig.KeepAliveInterval = d.KeepAliveInterval
	}
	if d.KeepAliveTimeout > 0 {
		smuxConfig.KeepAliveTimeout = d.KeepAliveTimeout
	}
	session, err := smux.Client(conn, smuxConfig)
	if err != nil {
		return nil, err
	}
	return &connAndSession{
		conn:    conn,
		session: session,
		dialer:  d,
		addr:    addr,
		idx:     idx,
	}, nil
}

func (cs *connAndSession) closeIfNecessary() {
	if cs.session.NumStreams() == 0 {
		// Closing session also closes connection
		cs.session.Close()
		cs.dialer.removeFromPool(cs.addr, cs.idx)
	}
}

func (d *dialer) removeFromPool(addr string, idx int) {
	d.mx.Lock()
	defer d.mx.Unlock()
	conns := d.pool[addr]
	if conns != nil {
		delete(conns, idx)
		if len(conns) == 0 {
			delete(d.pool, addr)
		}
	}
}
