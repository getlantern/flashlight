package gonat

import (
	"io"
	"net"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/oxtoacart/bpool"
)

const (
	// DefaultBufferPoolSize is 10 MB
	DefaultBufferPoolSize = 10000000

	// DefaultBufferDepth is 250 packets
	DefaultBufferDepth = 250

	// DefaultIdleTimeout is 65 seconds
	DefaultIdleTimeout = 65 * time.Second

	// DefaultStatsInterval is 15 seconds
	DefaultStatsInterval = 15 * time.Second

	// MinConntrackTimeout sets a lower bound on how long we'll let conntrack entries persist
	MinConntrackTimeout = 1 * time.Minute

	// MaximumIPPacketSize is 65535 bytes
	MaximumIPPacketSize = 65535
)

const (
	minEphemeralPort  = 32768
	maxEphemeralPort  = 61000 // consistent with most Linux kernels
	numEphemeralPorts = maxEphemeralPort - minEphemeralPort
)

var (
	log = golog.LoggerFor("gonat")
)

type Server interface {
	// Serve starts processing packets and blocks until finished
	Serve() error

	// Close stops the server and cleans up resources
	Close() error
}

// ReadWriter is like io.ReadWriter but using bpool.ByteSlice.
type ReadWriter interface {
	// Read reads data into a ByteSlice
	Read(bpool.ByteSlice) (int, error)

	// Write writes data from a ByteSlice
	Write(bpool.ByteSlice) (int, error)
}

// ReadWriterAdapter adapts io.ReadWriter to ReadWriter
type ReadWriterAdapter struct {
	io.ReadWriter
}

func (rw *ReadWriterAdapter) Read(b bpool.ByteSlice) (int, error) {
	return rw.ReadWriter.Read(b.Bytes())
}

func (rw *ReadWriterAdapter) Write(b bpool.ByteSlice) (int, error) {
	return rw.ReadWriter.Write(b.Bytes())
}

type Opts struct {
	// IFName is the name of the interface to use for connecting upstream.
	// If not specified, this will use the default interface for reaching the
	// Internet.
	IFName string

	// IFAddr is the address to use for outbound packets. Overrides the IFName
	// when specified.
	IFAddr string

	// BufferPool is a pool for buffers. If not provided, default to a 10MB pool.
	// Each []byte in the buffer pool should be <MaximumIPPacketSize> bytes.
	BufferPool bpool.ByteSlicePool

	// BufferDepth specifies the number of outbound packets to buffer between
	// stages in the send/receive pipeline. The default is <DefaultBufferDepth>.
	BufferDepth int

	// IdleTimeout specifies the amount of time before idle connections are
	// automatically closed. The default is <DefaultIdleTimeout>.
	IdleTimeout time.Duration

	// StatsTracker allows specifying an existing StatsTracker to use for tracking
	// stats. If not specified, one will be created using the configured StatsInterval.
	// Note - the StatsTracker has to be manually closed using its Close() method, otherwise
	// it will keep logging stats.
	StatsTracker *StatsTracker

	// StatsInterval controls how frequently to display stats. Defaults to
	// <DefaultStatsInterval>.
	StatsInterval time.Duration

	// OnOutbound allows modifying outbound ip packets.
	OnOutbound func(pkt *IPPacket)

	// OnInbound allows modifying inbound ip packets. ft is the 5 tuple to
	// which the current connection/UDP port mapping is keyed.
	OnInbound func(pkt *IPPacket, downFT FiveTuple)
}

// ApplyDefaults applies the default values to the given Opts, including making
// a new Opts if opts is nil.
func (opts *Opts) ApplyDefaults() error {
	if opts == nil {
		opts = &Opts{}
	}
	if opts.BufferPool == nil {
		opts.BufferPool = bpool.NewByteSlicePool(DefaultBufferPoolSize/MaximumIPPacketSize, MaximumIPPacketSize)
	}
	if opts.BufferDepth <= 0 {
		opts.BufferDepth = DefaultBufferDepth
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	if opts.StatsInterval <= 0 {
		opts.StatsInterval = DefaultStatsInterval
	}
	if opts.StatsTracker == nil {
		opts.StatsTracker = NewStatsTracker(opts.StatsInterval)
	}
	if opts.OnOutbound == nil {
		opts.OnOutbound = func(pkt *IPPacket) {}
	}
	if opts.OnInbound == nil {
		opts.OnInbound = func(pkt *IPPacket, downFT FiveTuple) {}
	}
	if opts.IFAddr == "" {
		var err error
		if opts.IFName != "" {
			opts.IFAddr, err = firstIPv4AddrFor(opts.IFName)
		} else {
			opts.IFAddr, err = findDefaultIPv4Addr()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func firstIPv4AddrFor(ifName string) (string, error) {
	outIF, err := net.InterfaceByName(ifName)
	if err != nil {
		return "", errors.New("Unable to find interface for interface %v: %v", ifName, err)
	}
	outIFAddrs, err := outIF.Addrs()
	if err != nil {
		return "", errors.New("Unable to get addresses for interface %v: %v", ifName, err)
	}
	for _, outIFAddr := range outIFAddrs {
		switch t := outIFAddr.(type) {
		case *net.IPNet:
			ipv4 := t.IP.To4()
			if ipv4 != nil {
				return ipv4.String(), nil
			}
		}
	}
	return "", errors.New("Unable to find IPv4 address for interface %v", ifName)
}

// findDefaultIPv4Addr find the default IPv4 address through which the Internet can be reached.
func findDefaultIPv4Addr() (string, error) {
	// try to find default interface by dialing an external connection
	conn, err := net.Dial("udp4", "lantern.io:80")
	if err != nil {
		return "", errors.New("Unable to dial lantern.io: %v", err)
	}
	ip := conn.LocalAddr().(*net.UDPAddr).IP.String()
	return ip, nil
}
