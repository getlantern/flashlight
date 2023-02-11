package darkstar

import (
	"crypto/rand"
	"errors"
	"net"
	"time"
)

type BlackHoleConn struct {
	timer  *time.Timer
	isOpen bool
}

func NewBlackHoleConn() *BlackHoleConn {
	duration, parseError := time.ParseDuration("30s")
	if parseError != nil {
		return nil
	}
	timer := time.NewTimer(duration)

	conn := &BlackHoleConn{timer, true}

	go func() {
		<-timer.C
		_ = conn.Close()
	}()

	return conn
}

func (b2 *BlackHoleConn) Read(b []byte) (n int, err error) {
	if b2.isOpen {
		return rand.Read(b)
	} else {
		return 0, errors.New("connection closed")
	}
}

func (b2 *BlackHoleConn) Write(b []byte) (n int, err error) {
	if b2.isOpen {
		return len(b), nil
	} else {
		return 0, errors.New("connection closed")
	}
}

func (b2 *BlackHoleConn) Close() error {
	if b2.isOpen {
		b2.isOpen = false
		b2.timer.Stop()
		return nil
	} else {
		return errors.New("connection closed")
	}
}

func (b2 *BlackHoleConn) LocalAddr() net.Addr {
	return nil
}

func (b2 *BlackHoleConn) RemoteAddr() net.Addr {
	return nil
}

func (b2 *BlackHoleConn) SetDeadline(_ time.Time) error {
	return nil
}

func (b2 *BlackHoleConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (b2 *BlackHoleConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

var _ net.Conn = &BlackHoleConn{nil, true}
