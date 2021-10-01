package client

import (
	"net"
	"sync/atomic"
	"time"
)

// optimisticListener is a listener that will never return an error from Accept but will
// instead keep trying (unless it has been closed)
type optimisticListener struct {
	net.Listener
	closed int64
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
		if atomic.LoadInt64(&l.closed) == 1 {
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
	atomic.StoreInt64(&l.closed, 1)
	return l.Listener.Close()
}
