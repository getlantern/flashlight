package chained

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/getlantern/golog"
)

type preconnectedConn struct {
	net.Conn
	expiresAt time.Time
}

func (pc *preconnectedConn) expired() bool {
	return pc.expiresAt.Before(time.Now())
}

type preconnectingDialer struct {
	log           golog.Logger
	maxPreconnect int
	expiration    time.Duration
	origDial      dialServerFn
	pool          chan *preconnectedConn
	closeCh       chan bool
}

func newPreconnectingDialer(name string, maxPreconnect int, expiration time.Duration, closeCh chan bool, origDial dialServerFn) *preconnectingDialer {
	pd := &preconnectingDialer{
		log:           golog.LoggerFor(fmt.Sprintf("chained.preconnect.%v", name)),
		origDial:      origDial,
		maxPreconnect: maxPreconnect,
		expiration:    expiration,
		pool:          make(chan *preconnectedConn, maxPreconnect*2),
		closeCh:       closeCh,
	}
	pd.log.Debugf("will preconnect up to %d times", maxPreconnect*2)
	go pd.closeWhenNecessary()
	return pd
}

func (pd *preconnectingDialer) dial(ctx context.Context, p *proxy) (conn net.Conn, err error) {
	// Whenever we dial successfully, warm up the pool by preconnecting
	defer func() {
		if err == nil {
			pd.preconnectIfNecessary(p)
		}
	}()

	// Try to get an unexpired preconnected connection when possible
	for {
		select {
		case pconn := <-pd.pool:
			if !pconn.expired() {
				pd.log.Tracef("using preconnection")
				conn = pconn.Conn
				return
			}
			pd.log.Tracef("preconnection expired before use")
		default:
			pd.log.Tracef("dialing on demand")
			conn, err = pd.origDial(ctx, p)
			return
		}
	}
}

func (pd *preconnectingDialer) preconnectIfNecessary(p *proxy) {
	if len(pd.pool) >= pd.maxPreconnect {
		// pool already full, don't bother
		// note - this check does not precisely bound the pool, but it's okay if we go a little over
		return
	}

	go func() {
		select {
		case <-pd.closeCh:
			pd.log.Trace("already closed, refusing to preconnect")
			return
		default:
			pd.preconnect(p)
		}
	}()
}

func (pd *preconnectingDialer) preconnect(p *proxy) {
	ctx, cancel := context.WithTimeout(context.Background(), chainedDialTimeout)
	defer cancel()

	expiration := time.Now().Add(pd.expiration)
	conn, err := pd.origDial(ctx, p)
	if err != nil {
		pd.log.Errorf("error preconnecting: %v", err)
		return
	}
	select {
	case pd.pool <- &preconnectedConn{conn, expiration}:
		pd.log.Trace("preconnected")
	default:
		// pool filled while we were dialing, just close conn and discard
		pd.log.Trace("discarding preconnection")
		conn.Close()
	}
}

func (pd *preconnectingDialer) closeWhenNecessary() {
	pd.log.Trace("waiting for close")
	// wait for close
	<-pd.closeCh

	for {
		select {
		case pconn := <-pd.pool:
			pd.log.Trace("closing preconnection")
			pconn.Conn.Close()
		case <-time.After(chainedDialTimeout * 2):
			pd.log.Trace("waited twice the chained dial timeout, no more preconnections to close")
			return
		}
	}
}
