package client

import (
	"net"

	"github.com/getlantern/flashlight/ops"
)

// Used by the Client to track operations on a per-connection basis.
type opsMap map[string]*ops.Op

func (m opsMap) put(downstream net.Conn, op *ops.Op) {
	m[downstream.RemoteAddr().String()] = op
}

func (m opsMap) get(downstream net.Conn) (op *ops.Op, ok bool) {
	op, ok = m[downstream.RemoteAddr().String()]
	return
}

func (m opsMap) delete(downstream net.Conn) {
	delete(m, downstream.RemoteAddr().String())
}
