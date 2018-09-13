package balancer

import (
	"io"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

type EchoConn struct{ b []byte }

func (e *EchoConn) Read(b []byte) (n int, err error) {
	return copy(b, e.b), nil
}

func (e *EchoConn) Write(b []byte) (n int, err error) {
	n = copy(e.b, b)
	e.b = e.b[:n]
	return n, nil
}

func (e *EchoConn) Close() error                             { return nil }
func (e *EchoConn) LocalAddr() net.Addr                      { return nil }
func (e *EchoConn) RemoteAddr() net.Addr                     { return nil }
func (e *EchoConn) SetDeadline(t time.Time) (err error)      { return nil }
func (e *EchoConn) SetReadDeadline(t time.Time) (err error)  { return nil }
func (e *EchoConn) SetWriteDeadline(t time.Time) (err error) { return nil }

func echoServer() (addr string, l net.Listener) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("Unable to listen: %s", err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err == nil {
				go func() {
					_, err = io.Copy(c, c)
					if err != nil {
						log.Fatalf("Unable to echo: %s", err)
					}
				}()
			}
		}
	}()
	addr = l.Addr().String()
	return
}
