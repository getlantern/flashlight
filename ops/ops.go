// Package ops wraps github.com/getlantern/ops with convenience methods
// for flashlight
package ops

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/ops"
)

// ProxyType is the type of various proxy channel
type ProxyType string

const (
	// ProxyNone means direct access, not proxying at all
	ProxyNone ProxyType = "none"
	// ProxyChained means access through Lantern hosted chained server
	ProxyChained ProxyType = "chained"
	// ProxyFronted means access through domain fronting
	ProxyFronted ProxyType = "fronted"
)

// Op decorates an ops.Op with convenience methods.
type Op struct {
	wrapped ops.Op
}

func (op *Op) Wrapped() ops.Op {
	return op.wrapped
}

// Begin mimics the similar method from ops.Op
func (op *Op) Begin(name string) *Op {
	return &Op{op.wrapped.Begin(name)}
}

// Begin mimics the similar method from ops
func Begin(name string) *Op {
	return &Op{ops.Begin(name)}
}

// RegisterReporter mimics the similar method from ops
func RegisterReporter(reporter ops.Reporter) {
	ops.RegisterReporter(reporter)
}

// Go mimics the similar method from ops.Op
func (op *Op) Go(fn func()) {
	op.wrapped.Go(fn)
}

// Go mimics the similar method from ops.
func Go(fn func()) {
	ops.Go(fn)
}

// End mimics the similar method from ops.Op
func (op *Op) End() {
	op.wrapped.End()
}

// Set mimics the similar method from ops.Op
func (op *Op) Set(key string, value interface{}) *Op {
	op.wrapped.Set(key, value)
	return op
}

// SetGlobal mimics the similar method from ops
func SetGlobal(key string, value interface{}) {
	ops.SetGlobal(key, value)
}

// SetDynamic mimics the similar method from ops.Op
func (op *Op) SetDynamic(key string, valueFN func() interface{}) *Op {
	op.wrapped.SetDynamic(key, valueFN)
	return op
}

// SetGlobalDynamic mimics the similar method from ops
func SetGlobalDynamic(key string, valueFN func() interface{}) {
	ops.SetGlobalDynamic(key, valueFN)
}

// FailIf mimics the similar method from ops.op
func (op *Op) FailIf(err error) error {
	return op.wrapped.FailIf(err)
}

// UserAgent attaches a user agent to the Context.
func (op *Op) UserAgent(v string) *Op {
	op.Set("user_agent", v)
	return op
}

// Request attaches key information of an `http.Request` to the Context.
func (op *Op) Request(req *http.Request) *Op {
	if req == nil {
		return op
	}
	op.Set("http_method", req.Method).
		Set("http_proto", req.Proto)
	op.Origin(req)
	return op
}

// Response attaches key information of an `http.Response` to the Context. If
// the response has corresponding Request it will call Request internally.
func (op *Op) Response(r *http.Response) *Op {
	if r == nil {
		return op
	}
	op.Set("http_response_status_code", r.StatusCode)
	op.Request(r.Request)
	return op
}

// ChainedProxy attaches chained proxy information to the Context
func (op *Op) ChainedProxy(addr string, protocol string) *Op {
	return op.ProxyType(ProxyChained).
		ProxyAddr(addr).
		ProxyProtocol(protocol)
}

// ProxyType attaches proxy type to the Context
func (op *Op) ProxyType(v ProxyType) *Op {
	return op.Set("proxy_type", v)
}

// ProxyAddr attaches proxy server address to the Context
func (op *Op) ProxyAddr(v string) *Op {
	host, port, err := net.SplitHostPort(v)
	if err == nil {
		op.Set("proxy_host", host).Set("proxy_port", port)
	}
	return op
}

// ProxyProtocol attaches proxy server's protocol to the Context
func (op *Op) ProxyProtocol(v string) *Op {
	return op.Set("proxy_protocol", v)
}

// Origin attaches the origin to the Contetx
func (op *Op) Origin(req *http.Request) *Op {
	op.Set("origin", req.Host)
	host, port, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	if (port == "0" || port == "") && req.Method != http.MethodConnect {
		port = "80"
	}
	op.Set("origin_host", host).Set("origin_port", port)
	return op
}

// DialTime records a dial time relative to a given start time (in milliseconds)
// and records whether or not the dial succeeded (based on err being nil).
func (op *Op) DialTime(start time.Time, err error) *Op {
	delta := time.Now().Sub(start)
	return op.Set("dial_time", float64(delta.Nanoseconds())/1000000).Set("dial_succeeded", err == nil)
}
