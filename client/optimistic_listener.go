package client

import (
	"net"
	"time"
)

// optimisticListener is a listener that will never return an error from Accept but will
// instead keep trying
type optimisticListener struct {
	net.Listener
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
		log.Errorf("Error on accepting, will try again: %v", err)
		time.Sleep(wait)
		// backoff
		wait += wait
		if wait > maxWait {
			wait = maxWait
		}
	}
}
