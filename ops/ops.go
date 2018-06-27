// Package ops wraps github.com/getlantern/ops with convenience methods
// for flashlight
package ops

import (
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	borda "github.com/getlantern/borda/client"
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

var (
	proxyNameRegex = regexp.MustCompile(`(fp-([a-z0-9]+-)?([a-z0-9]+)-[0-9]{8}-[0-9]+)(-.+)?`)
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

// Cancel mimics the similar method from ops.Op
func (op *Op) Cancel() {
	op.wrapped.Cancel()
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
	op.OriginFromRequest(req)
	return op
}

// Response attaches key information of an `http.Response` to the Context. If
// the response has corresponding Request it will call Request internally.
func (op *Op) Response(r *http.Response) *Op {
	if r == nil {
		return op
	}
	op.HTTPStatusCode(r.StatusCode)
	op.Request(r.Request)
	return op
}

func (op *Op) HTTPStatusCode(code int) *Op {
	op.Set("http_status_code", code)
	return op
}

// ChainedProxy attaches chained proxy information to the Context
func (op *Op) ChainedProxy(name string, addr string, protocol string, network string) *Op {
	return op.ProxyType(ProxyChained).
		ProxyName(name).
		ProxyAddr(addr).
		ProxyProtocol(protocol).
		ProxyNetwork(network)
}

// ProxyType attaches proxy type to the Context
func (op *Op) ProxyType(v ProxyType) *Op {
	return op.Set("proxy_type", v)
}

// ProxyName attaches the name of the proxy and the inferred datacenter to the
// Context
func (op *Op) ProxyName(name string) *Op {
	proxyName, dc := ProxyNameAndDC(name)
	if proxyName != "" {
		op.Set("proxy_name", proxyName)
	}
	if dc != "" {
		op.Set("dc", dc)
	}
	return op
}

// ProxyNameAndDC extracts the canonical proxy name and datacenter from a given
// full proxy name.
func ProxyNameAndDC(name string) (string, string) {
	match := proxyNameRegex.FindStringSubmatch(name)
	if len(match) == 5 {
		return match[1], match[3]
	}
	return "", ""
}

// ProxyAddr attaches proxy server address to the Context
func (op *Op) ProxyAddr(v string) *Op {
	host, port, err := net.SplitHostPort(v)
	if err == nil {
		op.Set("proxy_host", host).Set("proxy_port", port)
	}
	return op
}

// ProxyProtocol attaches proxy server's protocol (http, https or obfs4) to the Context
func (op *Op) ProxyProtocol(v string) *Op {
	return op.Set("proxy_protocol", v)
}

// ProxyNetwork attaches proxy server's network (tcp or kcp) to the Context
func (op *Op) ProxyNetwork(v string) *Op {
	return op.Set("proxy_network", v)
}

// OriginFromRequest attaches the origin to the Context based on the request's
// Host property.
func (op *Op) OriginFromRequest(req *http.Request) *Op {
	defaultPort := ""
	if req.Method != http.MethodConnect {
		defaultPort = "80"
	}
	return op.Origin(req.Host, defaultPort)
}

// Origin attaches the origin to the Context
func (op *Op) Origin(origin string, defaultPort string) *Op {
	op.Set("origin", origin)
	host, port, _ := net.SplitHostPort(origin)
	if host == "" && !strings.Contains(origin, ":") {
		host = origin
	}
	if port == "0" || port == "" {
		port = defaultPort
	}
	op.Set("origin_host", host).Set("origin_port", port)
	return op
}

// DialTime records a dial time relative to a given start time (in milliseconds)
// if and only if there is no error.
func (op *Op) DialTime(elapsed time.Duration, err error) *Op {
	if err != nil {
		return op
	}
	return op.SetMetric("dial_time", borda.Avg(float64(elapsed.Nanoseconds())/1000000))
}

// BalancerDialTime records a balancer dial time relative to a given start time (in
// milliseconds) if and only if there is no error.
func (op *Op) BalancerDialTime(elapsed time.Duration, err error) *Op {
	if err != nil {
		return op
	}
	return op.SetMetric("balancer_dial_time", borda.Avg(float64(elapsed.Nanoseconds())/1000000))
}

// CoreDialTime records a core dial time relative to a given start time (in
// milliseconds) if and only if there is no error.
func (op *Op) CoreDialTime(elapsed time.Duration, err error) *Op {
	if err != nil {
		return op
	}
	return op.SetMetric("core_dial_time", borda.Avg(float64(elapsed.Nanoseconds())/1000000))
}

// SetMetric sets a named metric. Metrics will be reported as borda values
// rather than dimensions.
func (op *Op) SetMetric(name string, value borda.Val) *Op {
	return op.Set(name, value)
}

func (op *Op) SetMetricSum(name string, value float64) *Op {
	return op.Set(name, borda.Sum(value))
}

func (op *Op) SetMetricMin(name string, value float64) *Op {
	return op.Set(name, borda.Min(value))
}

func (op *Op) SetMetricMax(name string, value float64) *Op {
	return op.Set(name, borda.Max(value))
}

func (op *Op) SetMetricAvg(name string, value float64) *Op {
	return op.Set(name, borda.Avg(value))
}
