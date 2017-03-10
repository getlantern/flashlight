// Package protected is used for creating "protected" connections
// that bypass Android's VpnService
package protected

import (
	"context"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
)

var (
	log = golog.LoggerFor("lantern-android.protected")
)

const (
	dnsServer      = "127.0.0.1"
	connectTimeOut = 15 * time.Second
	readDeadline   = 15 * time.Second
	writeDeadline  = 15 * time.Second
	socketError    = -1
	dnsPort        = 7300
)

type Protect func(fileDescriptor int) error

type ProtectedConn struct {
	net.Conn
	mutex    sync.Mutex
	isClosed bool
	socketFd int
	ip       [4]byte
	port     int
}

type Protector struct {
	protect Protect
}

func New(protect Protect) *Protector {
	return &Protector{protect}
}

// Resolve resolves the given address using a DNS lookup on a UDP socket
// protected by the given Protect function.
func (p *Protector) Resolve(network string, addr string) (*net.TCPAddr, error) {
	op := ops.Begin("protected-resolve").Set("addr", addr)
	defer op.End()
	conn, err := p.resolve(op, network, addr)
	return conn, op.FailIf(err)
}

func (p *Protector) resolve(op ops.Op, network string, addr string) (*net.TCPAddr, error) {
	host, port, err := SplitHostPort(addr)
	if err != nil {
		log.Errorf("Could not split host port: %v", err)
		return nil, err
	}

	// Check if we already have the IP address
	IPAddr := net.ParseIP(host)
	if IPAddr != nil {
		log.Debugf("Already have IP address; IP %v Port %v", IPAddr, port)
		return &net.TCPAddr{IP: IPAddr, Port: port}, nil
	}

	// Create a datagram socket
	socketFd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		err = errors.New("Error creating socket: %v", err)
		log.Error(err.Error())
		return nil, err
	}
	defer syscall.Close(socketFd)

	// Here we protect the underlying socket from the
	// VPN connection by passing the file descriptor
	// back to Java for exclusion
	err = p.protect(socketFd)
	if err != nil {
		err = errors.New("Could not bind socket to system device: %v", err)
		log.Error(err.Error())
		return nil, err
	}

	IPAddr = net.ParseIP(dnsServer)
	if IPAddr == nil {
		err = errors.New("Invalid IP address: %v", dnsServer)
		log.Error(err.Error())
		return nil, err
	}

	var ip [4]byte
	copy(ip[:], IPAddr.To4())
	sockAddr := syscall.SockaddrInet4{Addr: ip, Port: dnsPort}

	err = syscall.Connect(socketFd, &sockAddr)
	if err != nil {
		return nil, err
	}

	fd := uintptr(socketFd)
	file := os.NewFile(fd, "")
	defer file.Close()

	// return a copy of the network connection
	// represented by file
	fileConn, err := net.FileConn(file)
	if err != nil {
		log.Errorf("Error returning a copy of the network connection: %v", err)
		return nil, err
	}

	setQueryTimeouts(fileConn)

	result, err := dnsLookup(host, fileConn)
	if err != nil {
		log.Errorf("Error doing DNS resolution: %v", err)
		return nil, err
	}
	ipAddr, err := result.PickRandomIP()
	if err != nil {
		log.Errorf("No IP address available: %v", err)
		return nil, err
	}
	return &net.TCPAddr{IP: ipAddr, Port: port}, nil
}

// Dial creates a new protected connection.
// - syscall API calls are used to create and bind to the
//   specified system device (this is primarily
//   used for Android VpnService routing functionality)
func (p *Protector) Dial(network, addr string, timeout time.Duration) (net.Conn, error) {
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return p.DialContext(ctx, network, addr)
}

// DialContext is same as Dial, but accepts a context instead of timeout value.
func (p *Protector) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	op := ops.Begin("protected-dial").Set("addr", addr)
	dl, ok := ctx.Deadline()
	if ok {
		op.Set("timeout", dl.Sub(time.Now()).Seconds())
	}
	defer op.End()

	// Dial in goroutine to support arbitrary cancellation.
	var conn net.Conn
	var err error
	chDone := make(chan bool)
	go func() {
		conn, err = p.dialContext(op, ctx, network, addr)
		chDone <- true
	}()
	select {
	case <-ctx.Done():
		go func() {
			<-chDone
			if conn != nil {
				conn.Close()
			}
		}()
		return nil, op.FailIf(ctx.Err())
	case <-chDone:
		return conn, op.FailIf(err)
	}
}

// dialContext checks if context has been done between each phase to avoid
// unnecessary work, but doesn't support arbitrary cancellation.
func (p *Protector) dialContext(op ops.Op, ctx context.Context, network, addr string) (net.Conn, error) {
	socketType := 0
	switch network {
	case "tcp", "tcp4", "tcp6":
		socketType = syscall.SOCK_STREAM
	case "udp", "udp4", "udp6":
		socketType = syscall.SOCK_DGRAM
	default:
		err := errors.New("Unsupported network: %v", network)
		log.Error(err.Error())
		return nil, err
	}
	host, port, err := SplitHostPort(addr)
	if err != nil {
		log.Errorf("Could not split host port: %v", err)
		return nil, err
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Try to resolve it
		addr, err := p.Resolve(network, addr)
		if err != nil {
			log.Debugf("Could not resolve name: %v", err)
			return nil, err
		}
		ip = addr.IP
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	conn := &ProtectedConn{
		port: port,
	}
	copy(conn.ip[:], ip.To4())

	socketFd, err := syscall.Socket(syscall.AF_INET, socketType, 0)
	if err != nil {
		err = errors.New("Could not create socket: %v", err)
		log.Error(err.Error())
		return nil, err
	}
	conn.socketFd = socketFd
	defer conn.cleanup()

	// Actually protect the underlying socket here
	err = p.protect(conn.socketFd)
	if err != nil {
		err = errors.New("Unable to protect socket to %v: %v", addr, err)
		log.Error(err.Error())
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Actually connect the underlying socket
	err = conn.connectSocket()
	if err != nil {
		err = errors.New("Unable to connect socket to %v: %v", addr, err)
		log.Error(err.Error())
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// finally, convert the socket fd to a net.Conn
	err = conn.convert()
	if err != nil {
		return nil, errors.New("Error converting protected connection: %v", err)
	}
	return conn.Conn, nil
}

// connectSocket makes the connection to the given IP address port
// for the given socket fd
func (conn *ProtectedConn) connectSocket() error {
	sockAddr := syscall.SockaddrInet4{Addr: conn.ip, Port: conn.port}
	errCh := make(chan error, 2)
	time.AfterFunc(connectTimeOut, func() {
		errCh <- errors.New("connect timeout")
	})
	go func() {
		errCh <- syscall.Connect(conn.socketFd, &sockAddr)
	}()
	err := <-errCh
	return err
}

// converts the protected connection specified by
// socket fd to a net.Conn
func (conn *ProtectedConn) convert() error {
	conn.mutex.Lock()
	file := os.NewFile(uintptr(conn.socketFd), "")
	// dup the fd and return a copy
	fileConn, err := net.FileConn(file)
	// closes the original fd
	file.Close()
	conn.socketFd = socketError
	if err != nil {
		log.Errorf("Could not convert protected conn socket fd to a net.Conn: %v", err)
		conn.mutex.Unlock()
		return err
	}
	conn.Conn = fileConn
	conn.mutex.Unlock()
	return nil
}

// cleanup is run whenever we encounter a socket error
// we use a mutex since this connection is active in a variety
// of goroutines and to prevent any possible race conditions
func (conn *ProtectedConn) cleanup() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.socketFd != socketError {
		syscall.Close(conn.socketFd)
		conn.socketFd = socketError
	}
}

// Close is used to destroy a protected connection
func (conn *ProtectedConn) Close() (err error) {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if !conn.isClosed {
		conn.isClosed = true
		if conn.Conn == nil {
			if conn.socketFd == socketError {
				err = nil
			} else {
				err = syscall.Close(conn.socketFd)
				// update socket fd to socketError
				// to make it explicit this connection
				// has been closed
				conn.socketFd = socketError
			}
		} else {
			err = conn.Conn.Close()
		}
	}
	return err
}

// configure DNS query expiration
func setQueryTimeouts(c net.Conn) {
	now := time.Now()
	c.SetReadDeadline(now.Add(readDeadline))
	c.SetWriteDeadline(now.Add(writeDeadline))
}

// SplitHostAndPort is a wrapper around net.SplitHostPort that also uses strconv
// to convert the port to an int
func SplitHostPort(addr string) (string, int, error) {
	host, sPort, err := net.SplitHostPort(addr)
	if err != nil {
		log.Errorf("Could not split network address: %v", err)
		return "", 0, err
	}
	port, err := strconv.Atoi(sPort)
	if err != nil {
		log.Errorf("No port number found %v", err)
		return "", 0, err
	}
	return host, port, nil
}
