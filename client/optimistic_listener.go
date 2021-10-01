package client

import (
	"net"
	"sync"
	"time"
)

// optimisticListener is a listener that will never return an error from Accept but will
// instead keep trying (unless it has been closed)
type optimisticListener struct {
	net.Listener
	closed   bool
	closedMx sync.Mutex
}

func (l *optimisticListener) Accept() (net.Conn, error) {
	minWait := 10 * time.Millisecond
	maxWait := 1 * time.Second
	wait := minWait
	for {
		conn, err := l.Listener.Accept()
		if err == nil {
			wait = minWait
			return conn, nil
		}
		l.closedMx.Lock()
		closed := l.closed
		l.closedMx.Unlock()
		if closed {
			return nil, err
		}
		log.Errorf("Error on accepting, will try again: %v", err)
		time.Sleep(wait)
		// backoff
		wait += wait
		if wait > maxWait {
			wait = maxWait
		}
	}
}

func (l *optimisticListener) Close() error {
	l.closedMx.Lock()
	defer l.closedMx.Unlock()

	l.closed = true
	return l.Listener.Close()
}
