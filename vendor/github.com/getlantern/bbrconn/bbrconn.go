// Package bbrconn provides a wrapper around net.Conn that exposes BBR
// congestion control information. This works only on Linux with Kernel 4.9.0 or
// newer installed.
package bbrconn

import (
	"net"
	"time"
)

type InfoCallback func(bytesWritten int, info *TCPInfo, bbrInfo *BBRInfo, err error)

type Conn interface {
	net.Conn

	// BytesWritten returns the number of bytes written to this connection
	BytesWritten() int

	// TCPInfo returns TCP connection info from the kernel
	TCPInfo() (*TCPInfo, error)

	// BBRInfo returns BBR congestion avoidance info from the kernel
	BBRInfo() (*BBRInfo, error)
}

type TCPInfo struct {
	SenderMSS           uint
	RTT                 time.Duration
	SysSegsOut          uint
	SysTotalRetransSegs uint
}

type BBRInfo struct {
	// MaxBW is the maximum bottleneck bandwidth in bits per seconds (bps)
	MaxBW uint64
}
