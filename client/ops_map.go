package client

import (
	"net"
	"sync"

	"github.com/getlantern/flashlight/ops"
)

// Used by the Client to track operations on a per-connection basis. Concurrency safe.
type opsMap struct {
	m  map[string]*ops.Op
	mx sync.Mutex
}

func newOpsMap() *opsMap {
	return &opsMap{map[string]*ops.Op{}, sync.Mutex{}}
}

func (m *opsMap) put(downstream net.Conn, op *ops.Op) {
	m.mx.Lock()
	m.m[downstream.RemoteAddr().String()] = op
	m.mx.Unlock()
}

func (m *opsMap) get(downstream net.Conn) (op *ops.Op, ok bool) {
	m.mx.Lock()
	op, ok = m.m[downstream.RemoteAddr().String()]
	m.mx.Unlock()
	return
}

func (m *opsMap) delete(downstream net.Conn) {
	m.mx.Lock()
	delete(m.m, downstream.RemoteAddr().String())
	m.mx.Unlock()
}
