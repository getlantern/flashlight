package chained

import (
	"context"
	"fmt"
	"net"
	"sync"
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
	preconnected  int
	preconnecting int
	statsMutex    sync.RWMutex
	closeCh       chan bool
}

func newPreconnectingDialer(name string, maxPreconnect int, expiration time.Duration, closeCh chan bool, origDial dialServerFn) *preconnectingDialer {
	pd := &preconnectingDialer{
		log:           golog.LoggerFor(fmt.Sprintf("chained.preconnect.%v", name)),
		origDial:      origDial,
		maxPreconnect: maxPreconnect,
		expiration:    expiration,
		pool:          make(chan *preconnectedConn, maxPreconnect),
		closeCh:       closeCh,
	}
	pd.log.Debugf("will preconnect up to %d times", maxPreconnect)
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
			pd.decrementPreconnected()
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
	pd.statsMutex.Lock()
	defer pd.statsMutex.Unlock()
	if pd.preconnected+pd.preconnecting >= pd.maxPreconnect {
		// pool potentially full once in-flight preconnectings succeed, don't bother preconnecting
		return
	}
	pd.preconnecting++

	go func() {
		select {
		case <-pd.closeCh:
			pd.log.Trace("already closed, refusing to preconnect")
			pd.decrementPreconnecting()
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
		pd.decrementPreconnecting()
		return
	}
	pd.preconnectingSucceeded()
	pd.pool <- &preconnectedConn{conn, expiration}
	pd.log.Trace("preconnected")
}

func (pd *preconnectingDialer) numPreconnecting() int {
	pd.statsMutex.RLock()
	defer pd.statsMutex.RUnlock()
	return pd.preconnecting
}

func (pd *preconnectingDialer) numPreconnected() int {
	pd.statsMutex.RLock()
	defer pd.statsMutex.RUnlock()
	return pd.preconnected
}

func (pd *preconnectingDialer) decrementPreconnecting() {
	pd.statsMutex.Lock()
	pd.preconnecting--
	pd.statsMutex.Unlock()
}

func (pd *preconnectingDialer) preconnectingSucceeded() {
	pd.statsMutex.Lock()
	pd.preconnecting--
	pd.preconnected++
	pd.statsMutex.Unlock()
}

func (pd *preconnectingDialer) decrementPreconnected() {
	pd.statsMutex.Lock()
	pd.preconnected--
	pd.statsMutex.Unlock()
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
