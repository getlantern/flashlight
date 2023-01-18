// +build linux

package bbrconn

import (
	"fmt"
	"net"
	"reflect"
	"sync/atomic"

	"github.com/getlantern/netx"
	"github.com/mikioh/tcp"
	"github.com/mikioh/tcpinfo"
)

const (
	sizeOfTCPInfo    = 0xc0 // taken from github.com/mikioh/tcpinfo/syslinux.go
	sizeOfTCPBBRInfo = 0x14 // taken from github.com/mikioh/tcpinfo/syslinux.go
)

type bbrconn struct {
	net.Conn
	tconn        *tcp.Conn
	bytesWritten uint64
	onClose      InfoCallback
}

func (c *bbrconn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		atomic.AddUint64(&c.bytesWritten, uint64(n))
	}
	return n, err
}

// Wrapped implements the interface netx.Wrapped
func (c *bbrconn) Wrapped() net.Conn {
	return c.Conn
}

func Wrap(conn net.Conn, onClose InfoCallback) (Conn, error) {
	if onClose == nil {
		onClose = func(bytesWritten int, info *TCPInfo, bbrInfo *BBRInfo, err error) {
		}
	}
	var tcpConn net.Conn
	netx.WalkWrapped(conn, func(candidate net.Conn) bool {
		switch t := candidate.(type) {
		case *net.TCPConn:
			tcpConn = t
			return false
		}
		return true
	})
	if tcpConn == nil {
		return nil, fmt.Errorf("Could not find a net.TCPConn from connection of type %v", reflect.TypeOf(conn))
	}

	tconn, err := tcp.NewConn(tcpConn)
	if err != nil {
		return nil, fmt.Errorf("Unable to wrap TCP conn: %v", err)
	}
	return &bbrconn{Conn: conn, tconn: tconn, onClose: onClose}, nil
}

func (conn *bbrconn) BytesWritten() int {
	return int(atomic.LoadUint64(&conn.bytesWritten))
}

func (conn *bbrconn) TCPInfo() (*TCPInfo, error) {
	var o tcpinfo.Info
	b := make([]byte, sizeOfTCPInfo)
	i, err := conn.tconn.Option(o.Level(), o.Name(), b)
	if err != nil {
		return nil, err
	}
	info := i.(*tcpinfo.Info)
	return &TCPInfo{
		SenderMSS:           uint(info.SenderMSS),
		RTT:                 info.RTT,
		SysSegsOut:          info.Sys.SegsOut,
		SysTotalRetransSegs: info.Sys.TotalRetransSegs,
	}, nil
}

func (conn *bbrconn) BBRInfo() (*BBRInfo, error) {
	var o tcpinfo.CCInfo
	b := make([]byte, sizeOfTCPBBRInfo)
	i, err := conn.tconn.Option(o.Level(), o.Name(), b)
	if err != nil {
		return nil, err
	}
	ai, err := tcpinfo.ParseCCAlgorithmInfo("bbr", i.(*tcpinfo.CCInfo).Raw)
	if err != nil {
		return nil, err
	}
	bi := ai.(*tcpinfo.BBRInfo)
	return &BBRInfo{
		MaxBW: bi.MaxBW,
	}, nil
}

func (conn *bbrconn) Close() error {
	bytesWritten := conn.BytesWritten()
	info, err1 := conn.TCPInfo()
	bbrInfo, err2 := conn.BBRInfo()
	err := err1
	if err == nil {
		err = err2
	}
	conn.onClose(bytesWritten, info, bbrInfo, err)
	return conn.Conn.Close()
}
