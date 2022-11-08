package gonat

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/ops"
)

// newConn creates a connection built around a raw socket for either TCP or UDP
// (depending no the specified proto). Being a raw socket, it allows us to send our
// own IP packets.
func (s *server) newConn(downFT FiveTuple, upFT FiveTuple) (*conn, error) {
	socket, err := createSocket(upFT)
	if err != nil {
		return nil, err
	}
	c := &conn{
		ReadWriteCloser: socket,
		downFT:          downFT,
		upFT:            upFT,
		toUpstream:      make(chan *IPPacket, s.opts.BufferDepth),
		s:               s,
		closed:          make(chan interface{}),
	}
	ops.Go(c.writeToUpstream)
	return c, nil
}

func createSocket(upFT FiveTuple) (io.ReadWriteCloser, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, int(upFT.IPProto))
	if err != nil {
		return nil, errors.New("Unable to create transport: %v", err)
	}
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		syscall.Close(fd)
		return nil, errors.New("Unable to set IP_HDRINCL: %v", err)
	}
	bindAddr := sockAddrFor(upFT.Src)
	if err := syscall.Bind(fd, bindAddr); err != nil {
		syscall.Close(fd)
		return nil, errors.New("Unable to bind raw socket: %v", err)
	}
	if upFT.Dst.Port > 0 {
		connectAddr := sockAddrFor(upFT.Dst)
		if err := syscall.Connect(fd, connectAddr); err != nil {
			syscall.Close(fd)
			return nil, errors.New("Unable to connect raw socket: %v", err)
		}
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, errors.New("Unable to set raw socket to non-blocking: %v", err)
	}
	return os.NewFile(uintptr(fd), fmt.Sprintf("fd %d", fd)), nil
}

func sockAddrFor(addr Addr) syscall.Sockaddr {
	var ad [4]byte
	copy(ad[:], addr.IP())
	return &syscall.SockaddrInet4{
		Addr: ad,
		Port: int(addr.Port),
	}
}

type conn struct {
	io.ReadWriteCloser
	downFT     FiveTuple
	upFT       FiveTuple
	toUpstream chan *IPPacket
	s          *server
	lastActive int64
	closeOnce  sync.Once
	closed     chan interface{}
}

func (c *conn) writeToUpstream() {
	defer func() {
		c.s.closedConns <- c
	}()
	defer c.ReadWriteCloser.Close()

	for {
		select {
		case pkt := <-c.toUpstream:
			pkt.SetSource(c.upFT.Src)
			pkt.recalcChecksum()
			_, err := c.Write(pkt.Raw.Bytes())
			c.s.bufferPool.PutSlice(pkt.Raw)
			if err != nil {
				log.Errorf("Error writing upstream: %v", err)
				c.Close()
				// Wait for conn to have actually been closed
				<-c.closed
				return
			}
			c.markActive()
		case <-c.closed:
			return
		}
	}
}

func (c *conn) markActive() {
	atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
}

func (c *conn) timeSinceLastActive() time.Duration {
	return time.Duration(time.Now().UnixNano() - atomic.LoadInt64(&c.lastActive))
}

func (c *conn) Close() error {
	select {
	case <-c.closed:
		// already closed
	default:
		// Ask server to close this conn so that it happens in the dispatch loop
		c.s.requestCloseConn(c)
	}
	return nil
}

func (c *conn) doClose() bool {
	closed := false
	c.closeOnce.Do(func() {
		close(c.closed)
		closed = true
	})
	return closed
}
