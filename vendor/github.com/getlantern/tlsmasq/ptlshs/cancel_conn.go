package ptlshs

import (
	"errors"
	"net"
	"sync"
	"time"
)

type cancelledIOError struct{}

func (err cancelledIOError) Error() string   { return "cancelled" }
func (err cancelledIOError) Timeout() bool   { return false }
func (err cancelledIOError) Temporary() bool { return true }

// Wrap an existing net.Conn with newCancelConn and use normally. Unblock pending Reads or Writes
// using cancelIO(). There are no side-effects other than performance penalties due to locking.
type cancelConn struct {
	net.Conn
	sync.Mutex

	rDeadline, wDeadline time.Time
	pendingIO            int
	cancelErrors         chan error // non-nil iff a cancel is ongoing
	cancelComplete       *sync.Cond
}

func newCancelConn(conn net.Conn) *cancelConn {
	cc := &cancelConn{Conn: conn}
	cc.cancelComplete = sync.NewCond(&cc.Mutex)
	return cc
}

// When this function returns, conn.Lock will be held and any pending cancels will be complete.
func (conn *cancelConn) waitForPendingCancel() {
	conn.Lock()
	for conn.cancelErrors != nil {
		conn.cancelComplete.Wait()
	}
}

func (conn *cancelConn) SetReadDeadline(t time.Time) error {
	conn.waitForPendingCancel()
	err := conn.Conn.SetReadDeadline(t)
	if err == nil {
		conn.rDeadline = t
	}
	conn.Unlock()
	return err
}

func (conn *cancelConn) SetWriteDeadline(t time.Time) error {
	conn.waitForPendingCancel()
	err := conn.Conn.SetWriteDeadline(t)
	if err == nil {
		conn.wDeadline = t
	}
	conn.Unlock()
	return err
}

func (conn *cancelConn) SetDeadline(t time.Time) error {
	conn.waitForPendingCancel()
	err := conn.Conn.SetDeadline(t)
	if err == nil {
		conn.rDeadline, conn.wDeadline = t, t
	}
	conn.Unlock()
	return err
}

func (conn *cancelConn) doIO(b []byte, io func([]byte) (int, error)) (n int, err error) {
	conn.waitForPendingCancel()
	conn.pendingIO++
	conn.Unlock()

	n, err = io(b)
	conn.Lock()
	if conn.cancelErrors != nil {
		conn.cancelErrors <- err
		err = cancelledIOError{}
	}
	conn.pendingIO--
	conn.Unlock()
	return
}

func (conn *cancelConn) Read(b []byte) (n int, err error) {
	return conn.doIO(b, conn.Conn.Read)
}

func (conn *cancelConn) Write(b []byte) (n int, err error) {
	return conn.doIO(b, conn.Conn.Write)
}

// cancelIO cancels all pending I/O operations. Any blocked callers of Read or Write will receive
// errorCancelledIO.
func (conn *cancelConn) cancelIO() error {
	conn.Lock()
	rDeadline, wDeadline := conn.rDeadline, conn.wDeadline
	pendingIO := conn.pendingIO
	conn.Conn.SetDeadline(time.Now().Add(-1 * time.Second))
	conn.cancelErrors = make(chan error, conn.pendingIO)
	conn.Unlock()

	defer func() {
		conn.Lock()
		conn.Conn.SetReadDeadline(rDeadline)
		conn.Conn.SetWriteDeadline(wDeadline)
		conn.cancelErrors = nil
		conn.cancelComplete.Broadcast()
		conn.Unlock()
	}()

	var netErr net.Error
	for i := 0; i < pendingIO; i++ {
		err := <-conn.cancelErrors
		if err == nil {
			continue
		}
		if errors.As(err, &netErr) && !netErr.Timeout() {
			return err
		}
	}
	return nil
}
