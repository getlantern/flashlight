package proxyimpl

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
)

type Prefix []byte

func (p Prefix) String() string {
	return hex.EncodeToString(p)
}

func NewPrefixFromHexString(hexString string) (Prefix, error) {
	b, err := hex.DecodeString(hexString)
	if err != nil {
		return nil, fmt.Errorf("Unable to decode prefix %s: %s", hexString, err)
	}
	return b, nil
}

func NewPrefixSliceFromHexStringSlice(hexStrings []string) ([]Prefix, error) {
	prefixes := make([]Prefix, len(hexStrings))
	for i, hexString := range hexStrings {
		p, err := NewPrefixFromHexString(hexString)
		if err != nil {
			return nil, fmt.Errorf(
				"Unable to decode prefix %s: %s",
				hexString,
				err,
			)
		}
		prefixes[i] = p
	}
	return prefixes, nil
}

// PrefixImpl implements chained.proxyImpl interface.
//
// PrefixImpl is a really dummy implementation of proxyimpl.ProxyImpl that just
// wraps another proxyimpl.ProxyImpl. It doesn't override any of the ProxyImpl
// methods. It's primarily used to house the prefix logic and provide an easy
// location to access it.
//
// It's used now inside "chained/proxy.go:CreateDialer()": when the proxy YAML
// configuration we get from config-server has a "prefixes" field, we wrap the
// proxyimpl.ProxyImpl from a specific pluggable transport that was created in
// that function with a PrefixImpl.
//
// Any callers interested in receiving the prefixes that were successfully
// connected to can register a callback function with PrefixImpl using
// PrefixImpl.SetSuccessfulPrefixCallback().
//
// Later, when the dialing logic triggers (mostly in
// "chained.dialer.go:multipathPrefixDialOrigin()"), we'll access the
// PrefixImpl.PrefixImpl field to get the prefixes and use the callbacks
// registered with PrefixImpl to notify the caller of the successful prefixes.
//
// Some example code:
//
//	    // Create a balancer.Dialer
//	    dialer := chained.CreateDialer(...)
//		// Create a proxyimpl.ProxyImpl from the configuration we get from
//		// config-server
//		proxyImpl, _ := ...
//		// Fetch prefixes from config-server.
//		prefixes, _ := ...
//
//		// Create a new proxyimpl.ProxyImpl for the pluggable transport.
//		prefixImpl, _ := proxyimpl.NewPrefixImpl(proxyImpl, prefixes)
//		// Assign the proxyimpl.ProxyImpl to the balancer.Dialer.
//		dialer.impl = prefixImpl
//
//		// In some other place in the code, register the callback function
//		if prefixImpl, ok := dialer.Implementation().(*proxyimpl.PrefixImpl); ok {
//		  prefixImpl.SetSuccessfulPrefixCallback(func(pr proxyimpl.Prefix) {
//		    // Do something with the prefix.
//		  })
//		}
//
//		// In some other place in the code that dials to the proxy, send
//		// successfully-dialed prefixes to the callback function.
//		// In production, this happens in "chained.dialer.go:multipathPrefixDialOrigin()".
//		if prefixImpl, ok := dialer.Implementation().(*proxyimpl.PrefixImpl); ok {
//		  prefixImpl.ReceiveSuccessfulPrefix(pr)
//		})
type PrefixImpl struct {
	ProxyImpl
	Prefixes []Prefix

	successfulPrefixCallback func(p Prefix)
}

func NewPrefixImpl(
	p ProxyImpl,
	prefixes []Prefix,
) *PrefixImpl {
	return &PrefixImpl{
		ProxyImpl: p,
		Prefixes:  prefixes,
	}
}

func (p *PrefixImpl) SetSuccessfulPrefixCallback(fn func(Prefix)) {
	p.successfulPrefixCallback = fn
}

func (p *PrefixImpl) ReceiveSuccessfulPrefix(pr Prefix) {
	if p.successfulPrefixCallback == nil {
		return
	}
	// XXX <13-03-2023, soltzen> Do this in a goroutine so we don't worry about
	// blocking. We don't really care if we miss a prefix or the order of the
	// prefixes.
	go p.successfulPrefixCallback(pr)
}

type prefixConn struct {
	*net.TCPConn
	Prefix []byte

	prefixOnce sync.Once
}

func NewPrefixConn(c *net.TCPConn, prefix []byte) *prefixConn {
	return &prefixConn{
		TCPConn: c,
		Prefix:  prefix,
	}
}

func (c *prefixConn) Read(b []byte) (int, error) {
	return c.TCPConn.Read(b)
}

func (c *prefixConn) Write(b []byte) (int, error) {
	var err error
	var n int
	c.prefixOnce.Do(func() {
		fmt.Printf("Writing prefix: %s", b)
		n, err = c.TCPConn.Write(c.Prefix)
	})
	if err != nil {
		return n, fmt.Errorf("Unable to write prefix: %s", err)
	}
	return c.TCPConn.Write(b)
}

type PrefixTCPDialer struct {
	prefix    []byte
	localAddr net.Addr
}

func NewPrefixTCPDialer(prefix []byte) *PrefixTCPDialer {
	return &PrefixTCPDialer{
		prefix: prefix,
	}
}

func (d *PrefixTCPDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	laddr := d.localAddr.(*net.TCPAddr)
	raddr, err := net.ResolveTCPAddr(network, addr)
	if err != nil {
		return nil, fmt.Errorf("Unable to resolve TCP address: %s", err)
	}
	c, err := net.DialTCP("tcp", laddr, raddr)
	if err != nil {
		return nil, fmt.Errorf("Unable to dial proxy: %v", err)
	}

	prefixedConn := &prefixConn{
		TCPConn: c,
		Prefix:  d.prefix,
	}
	return prefixedConn, nil
}

func (d *PrefixTCPDialer) SetLocalAddr(addr net.Addr) {
	d.localAddr = addr
}
