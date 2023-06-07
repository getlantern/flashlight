// Package ops wraps github.com/getlantern/ops with convenience methods
// for flashlight
package ops

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/jibber_jabber"
	"github.com/getlantern/ops"
	"github.com/getlantern/osversion"

	"github.com/getlantern/flashlight/v7/bandwidth"
	"github.com/getlantern/flashlight/v7/common"
)

// ProxyType is the type of various proxy channel
type ProxyType string
type contextKey string

const (
	// ProxyNone means direct access, not proxying at all
	ProxyNone ProxyType = "none"
	// ProxyChained means access through Lantern hosted chained server
	ProxyChained ProxyType = "chained"
	// ProxyFronted means access through domain fronting
	ProxyFronted ProxyType = "fronted"

	// NB - a beam is like a tracing ID and has nothing to do with the application named "beam"
	CtxKeyBeam = contextKey("beam")
	ctxKeyOp   = contextKey("op")
)

var (
	beam_seq uint64
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

// WithOp adds an Op to the context to be retrieved later via FromContext
func WithOp(ctx context.Context, op *Op) context.Context {
	return context.WithValue(ctx, ctxKeyOp, op)
}

// FromContext extracts an Op from the context, or nil if doesn't exist
func FromContext(ctx context.Context) *Op {
	if op, exist := ctx.Value(ctxKeyOp).(*Op); exist {
		return op
	}
	return nil
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
func (op *Op) ChainedProxy(name string, addr string, protocol string, network string, multiplexed bool) *Op {
	return op.ProxyType(ProxyChained).
		ProxyName(name).
		ProxyAddr(addr).
		ProxyProtocol(protocol).
		ProxyNetwork(network).
		ProxyMultiplexed(multiplexed)
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
func ProxyNameAndDC(name string) (proxyName string, dc string) {
	hyphenIndexes := make([]int, 0, 5)
	for i, r := range name {
		if r == '-' {
			hyphenIndexes = append(hyphenIndexes, i)
		}
	}

	if len(hyphenIndexes) < 2 {
		return
	}

	dc = name[hyphenIndexes[0]+1 : hyphenIndexes[1]]
	if len(hyphenIndexes) > 3 {
		proxyName = name[:hyphenIndexes[3]]
	} else {
		proxyName = name
	}

	return
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

func (op *Op) ProxyMultiplexed(v bool) *Op {
	return op.Set("proxy_multiplexed", v)
}

// OriginFromRequest attaches the origin to the Context based on the request's
// Host property.
func (op *Op) OriginFromRequest(req *http.Request) *Op {
	defaultPort := ""
	if req.Method != http.MethodConnect {
		defaultPort = "80"
	}
	return op.OriginPort(req.Host, defaultPort)
}

// Origin attaches the origin to the Context
func (op *Op) OriginPort(origin string, defaultPort string) *Op {
	host, port, _ := net.SplitHostPort(origin)
	if host == "" && !strings.Contains(origin, ":") {
		host = origin
	}
	if port == "0" || port == "" {
		port = defaultPort
	}
	op.Set("origin_port", port)
	return op
}

// DialTime records a dial time relative to a given start time (in milliseconds)
// if and only if there is no error.
func (op *Op) DialTime(elapsed time.Duration, err error) *Op {
	if err != nil {
		return op
	}
	return op.SetMetricPercentile("dial_time", float64(elapsed.Nanoseconds())/1000000)
}

// BalancerDialTime records a balancer dial time relative to a given start time (in
// milliseconds) if and only if there is no error.
func (op *Op) BalancerDialTime(elapsed time.Duration, err error) *Op {
	if err != nil {
		return op
	}
	return op.SetMetricPercentile("balancer_dial_time", float64(elapsed.Nanoseconds())/1000000)
}

// CoreDialTime records a core dial time relative to a given start time (in
// milliseconds) if and only if there is no error.
func (op *Op) CoreDialTime(elapsed time.Duration, err error) *Op {
	if err != nil {
		return op
	}
	return op.SetMetricPercentile("core_dial_time", float64(elapsed.Nanoseconds())/1000000)
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

func (op *Op) SetMetricPercentile(name string, value float64) *Op {
	return op.Set(name, borda.Percentile(value))
}

// InitGlobalContext configures global context info
func InitGlobalContext(appName string, deviceID string, isPro func() bool, getCountry func() string) {
	// Using "application" allows us to distinguish between errors from the
	// lantern client vs other sources like the http-proxy, etop.
	ops.SetGlobal("app", fmt.Sprintf("%s-client", strings.ToLower(appName)))
	ops.SetGlobal("app_version", fmt.Sprintf("%v (%v)", common.ApplicationVersion, common.RevisionDate))
	ops.SetGlobal("go_version", runtime.Version())
	ops.SetGlobal("os_name", common.Platform)
	ops.SetGlobal("os_arch", runtime.GOARCH)
	ops.SetGlobal("device_id", deviceID)
	ops.SetGlobalDynamic("geo_country", func() interface{} { return getCountry() })
	ops.SetGlobalDynamic("timezone", func() interface{} { return time.Now().Format("MST") })
	ops.SetGlobalDynamic("locale_language", func() interface{} {
		lang, _ := jibber_jabber.DetectLanguage()
		return lang
	})
	ops.SetGlobalDynamic("locale_country", func() interface{} {
		country, _ := jibber_jabber.DetectTerritory()
		return country
	})
	ops.SetGlobalDynamic("is_pro", func() interface{} {
		return isPro()
	})
	ops.SetGlobalDynamic("is_data_capped", func() interface{} {
		if isPro() {
			return false
		}
		quota, _ := bandwidth.GetQuota()
		return quota != nil && quota.MiBUsed >= quota.MiBAllowed
	})

	if osStr, err := osversion.GetHumanReadable(); err == nil {
		ops.SetGlobal("os_version", osStr)
	}
}
