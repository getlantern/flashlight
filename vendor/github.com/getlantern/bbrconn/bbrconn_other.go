// +build !linux

package bbrconn

import (
	"net"
)

func Wrap(conn net.Conn, onClose InfoCallback) (Conn, error) {
	panic("bbrconn.Wrap only supported on Linux")
}
