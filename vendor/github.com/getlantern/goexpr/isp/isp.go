// Package isp provides isp lookup functions for IPv4 addresses
package isp

import (
	"fmt"
	"sync/atomic"

	"github.com/getlantern/goexpr"
	"github.com/getlantern/golog"
	"github.com/getlantern/msgpack"
)

const (
	// DefaultCacheSize determines the default size for the ip cache
	DefaultCacheSize = 100000
)

var (
	log = golog.LoggerFor("goexpr.isp")

	provider atomic.Value
)

func init() {
	msgpack.RegisterExt(100, &ispExpr{})
}

// Provider implements the actual looking up of ISP and ASN information.
type Provider interface {
	// ISP looks up the name of the Internet Service Provider corresponding to the
	// given ip.
	ISP(ip string) (isp string, found bool)

	// ORG looks up the name of the Organization corresponding to the given ip
	// (may be different than ISP).
	ORG(ip string) (org string, found bool)

	// ASN looks up the Autonomous System Number corresponding to the given ip.
	ASN(ip string) (asn int, found bool)

	// ASName looks up the Autonomous System Name corresponding to the given ip.
	ASName(ip string) (asnName string, found bool)
}

// SetProvider sets the ISP data provider
func SetProvider(prov Provider, cacheSize int) {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
		log.Debugf("Defaulted ip cache size to %v", cacheSize)
	}
	provider.Store(withCaching(prov, cacheSize))
}

func getProvider() Provider {
	return provider.Load().(Provider)
}

// ISP returns the ISP name for a given IPv4 address
func ISP(ip goexpr.Expr) goexpr.Expr {
	return &ispExpr{"ISP", ip, func(ip string) (interface{}, bool) {
		return getProvider().ISP(ip)
	}}
}

// ORG returns the Organization name for a given IPv4 address (similar to ISP
// but may have different data depending on provider used).
func ORG(ip goexpr.Expr) goexpr.Expr {
	return &ispExpr{"ORG", ip, func(ip string) (interface{}, bool) {
		return getProvider().ORG(ip)
	}}
}

// ASN returns the ASN number for a given IPv4 address as an int
func ASN(ip goexpr.Expr) goexpr.Expr {
	return &ispExpr{"ASN", ip, func(ip string) (interface{}, bool) {
		return getProvider().ASN(ip)
	}}
}

// ASName returns the ASN name for a given IPv4 address
func ASName(ip goexpr.Expr) goexpr.Expr {
	return &ispExpr{"ASNAME", ip, func(ip string) (interface{}, bool) {
		return getProvider().ASName(ip)
	}}
}

type ispExpr struct {
	Name string
	IP   goexpr.Expr
	Fn   func(ip string) (interface{}, bool)
}

func (e *ispExpr) Eval(params goexpr.Params) interface{} {
	_ip := e.IP.Eval(params)
	switch ip := _ip.(type) {
	case string:
		result, found := e.Fn(ip)
		if !found {
			return nil
		}
		return result
	}
	return nil
}

func (e *ispExpr) WalkParams(cb func(string)) {
	e.IP.WalkParams(cb)
}

func (e *ispExpr) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *ispExpr) WalkLists(cb func(goexpr.List)) {
	e.IP.WalkLists(cb)
}

func (e *ispExpr) String() string {
	return fmt.Sprintf("%v(%v)", e.Name, e.IP)
}
