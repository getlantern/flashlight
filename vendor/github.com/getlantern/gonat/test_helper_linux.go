package gonat

import (
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/fdcount"
	"github.com/getlantern/grtrack"

	"github.com/stretchr/testify/assert"
)

const (
	closeTimeout = 2 * time.Second
)

var (
	serverTCPConnections int64
)

// Note - this test has to be run with root permissions to allow setting up the
// TUN device.
func RunTest(t *testing.T, tunDeviceName, tunAddr, tunGW, tunMask string, mtu int, doTest func(ifAddr string, dev io.ReadWriter, origEchoAddr Addr, finishedCh chan interface{}) (func() error, error)) {
	_, tcpConnCount, err := fdcount.Matching("TCP")
	if !assert.NoError(t, err, "unable to get initial TCP socket count") {
		return
	}
	_, udpConnCount, err := fdcount.Matching("UDP")
	if !assert.NoError(t, err, "unable to get initial UDP socket count") {
		return
	}
	_, rawConnCount, err := fdcount.Matching("raw")
	if !assert.NoError(t, err, "unable to get initial raw socket count") {
		return
	}

	// Open a TUN device
	log.Debugf("Opening TUN device at %v", tunAddr)
	dev, err := TUNDevice(tunDeviceName, tunAddr, tunMask, mtu)
	if err != nil {
		if err != nil {
			if strings.HasSuffix(err.Error(), "operation not permitted") {
				t.Log("This test requires root access. Compile, then run with root privileges. See the README for more details.")
			}
			t.Fatal(err)
		}
	}
	// for some reason, on some Linux installs, reading hangs even after the device is closed.
	// withTimeout is a hack that allows to return an EOF to the reader before our test ends.
	dev = withTimeout(dev, closeTimeout)

	grtracker := grtrack.Start()

	opts := &Opts{}
	if err := opts.ApplyDefaults(); !assert.NoError(t, err) {
		return
	}

	// Start echo servers
	closeCh := make(chan interface{})
	echoAddr := tcpEcho(t, closeCh, opts.IFAddr)
	udpEcho(t, closeCh, echoAddr)
	host, _port, _ := net.SplitHostPort(echoAddr)
	port, _ := strconv.Atoi(_port)
	origEchoAddr := Addr{host, uint16(port)}
	echoAddr = tunGW + ":" + _port

	finishedCh := make(chan interface{})
	doClose, err := doTest(opts.IFAddr, dev, origEchoAddr, finishedCh)
	if !assert.NoError(t, err) {
		return
	}

	b := make([]byte, 8)
	log.Debugf("Dialing echo server with UDP at: %v", echoAddr)
	uconn, err := net.Dial("udp", echoAddr)
	if !assert.NoError(t, err, "Unable to get UDP connection to TUN device") {
		return
	}

	log.Debug("Writing to UDP")
	_, err = uconn.Write([]byte("helloudp"))
	if !assert.NoError(t, err) {
		return
	}

	log.Debug("Reading from UDP")
	_, err = io.ReadFull(uconn, b)
	if !assert.NoError(t, err) {
		return
	}

	log.Debug("Closing udp conn")
	uconn.Close()
	log.Debug("Closed udp conn")

	log.Debugf("Dialing echo server with TCP at: %v", echoAddr)
	conn, err := net.DialTimeout("tcp", echoAddr, 5*time.Second)
	if !assert.NoError(t, err) {
		return
	}

	_, err = conn.Write([]byte("hellotcp"))
	if !assert.NoError(t, err) {
		return
	}

	_, err = io.ReadFull(conn, b)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "hellotcp", string(b))
	conn.Close()
	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, atomic.LoadInt64(&serverTCPConnections), "Server-side TCP connection should have been closed")

	close(closeCh)
	if err := dev.Close(); !assert.NoError(t, err) {
		return
	}
	doClose()

	time.Sleep(2 * closeTimeout)
	select {
	case <-finishedCh:
		tcpConnCount.AssertDelta(0)
		udpConnCount.AssertDelta(0)
		rawConnCount.AssertDelta(0)
		time.Sleep(2 * time.Second)
		grtracker.Check(t)
	case <-time.After(15 * time.Second):
		t.Error("gonat failed to terminate in a reasonable amount of time")
	}
}

func tcpEcho(t *testing.T, closeCh <-chan interface{}, ip string) string {
	l, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		<-closeCh
		l.Close()
	}()
	log.Debugf("TCP echo server listening at: %v", l.Addr())

	go func() {
		for {
			conn, err := l.Accept()
			log.Debugf("TCP echo accepted, error? %v", err)
			if err != nil {
				return
			}
			atomic.AddInt64(&serverTCPConnections, 1)
			go func() {
				io.Copy(conn, conn)
				conn.Close()
				atomic.AddInt64(&serverTCPConnections, -1)
			}()
		}
	}()

	return l.Addr().String()
}

func udpEcho(t *testing.T, closeCh <-chan interface{}, echoAddr string) {
	conn, err := net.ListenPacket("udp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		<-closeCh
		conn.Close()
	}()
	log.Debugf("UDP echo server listening at: %v", conn.LocalAddr())

	go func() {
		b := make([]byte, 20480)
		for {
			n, addr, err := conn.ReadFrom(b)
			if err != nil {
				return
			}
			conn.WriteTo(b[:n], addr)
		}
	}()
}

type read struct {
	b   []byte
	err error
}

func withTimeout(dev io.ReadWriteCloser, timeout time.Duration) io.ReadWriteCloser {
	result := &timingOutReader{
		ReadWriteCloser: dev,
		timeout:         timeout,
		reads:           make(chan *read),
	}
	go result.process()
	return result
}

type timingOutReader struct {
	io.ReadWriteCloser
	timeout time.Duration
	reads   chan *read
}

func (r *timingOutReader) process() {
	for {
		b := make([]byte, MaximumIPPacketSize)
		n, err := r.ReadWriteCloser.Read(b)
		r.reads <- &read{b[:n], err}
	}
}

func (r *timingOutReader) Read(b []byte) (n int, err error) {
	select {
	case read := <-r.reads:
		copy(b, read.b)
		return len(read.b), read.err
	case <-time.After(r.timeout):
		return 0, io.EOF
	}
}
