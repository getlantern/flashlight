package proxyimpl

import (
	"context"
	"net"
	"strings"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
	"github.com/getlantern/netx"
)

var log = golog.LoggerFor("proxyimpl")

// ProxyImpl is the interface to hide the details of client side logic for
// different types of pluggable transports.
type ProxyImpl interface {
	// DialServer is to establish connection to the proxy server to the point
	// of being able to transfer application data.
	DialServer(
		op *ops.Op,
		ctx context.Context,
		dialer Dialer,
	) (net.Conn, error)
	// close releases the resources associated with the implementation, if any.
	Close()
}

type ReportDialCoreFn func(op *ops.Op, dialCore func() (net.Conn, error)) (net.Conn, error)

type coreDialer func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error)

func New(
	configDir, name, addr, transport string,
	s *config.ProxyConfig,
	uc common.UserConfig,
	reportDialCore ReportDialCoreFn,
) (ProxyImpl, error) {
	coreDialer := func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error) {
		return reportDialCore(op, func() (net.Conn, error) {
			return netx.DialContext(ctx, "tcp", addr)
		})
	}
	if strings.HasPrefix(transport, "utp") {
		dialer, err := utpDialer()
		if err != nil {
			return nil, err
		}
		coreDialer = func(op *ops.Op, ctx context.Context, addr string) (net.Conn, error) {
			return reportDialCore(op, func() (net.Conn, error) {
				return dialer(ctx, addr)
			})
		}
	}
	var impl ProxyImpl
	var err error
	switch transport {
	case "", "http", "https", "utphttp", "utphttps":
		if s.Cert == "" {
			log.Debugf(
				"No Cert configured for %s, will dial with plain tcp",
				addr,
			)
			impl = newHTTPImpl(addr, coreDialer)
		} else {
			log.Tracef("Cert configured for %s, will dial with tls", addr)
			impl, err = newHTTPSImpl(configDir, name, addr, s, uc, coreDialer)
		}
	case "obfs4", "utpobfs4":
		impl, err = newOBFS4Impl(name, addr, s, coreDialer)
	case "lampshade":
		impl, err = newLampshadeImpl(name, addr, s, reportDialCore)
	case "quic_ietf":
		impl, err = newQUICImpl(name, addr, s, reportDialCore)
	case "shadowsocks":
		impl, err = newShadowsocksImpl(name, addr, s, reportDialCore)
	case "wss":
		impl, err = newWSSImpl(addr, s, reportDialCore)
	case "tlsmasq":
		impl, err = newTLSMasqImpl(configDir, name, addr, s, uc, reportDialCore)
	case "starbridge":
		impl, err = newStarbridgeImpl(name, addr, s, reportDialCore)
	default:
		err = errors.New("Unknown transport: %v", transport).
			With("addr", addr).
			With("plugabble-transport", transport)
	}
	if err != nil {
		return nil, err
	}

	allowPreconnecting := false
	switch transport {
	case "http", "https", "utphttp", "utphttps", "obfs4", "utpobfs4", "tlsmasq":
		allowPreconnecting = true
	}

	if s.MultiplexedAddr != "" || transport == "utphttp" ||
		transport == "utphttps" || transport == "utpobfs4" ||
		transport == "tlsmasq" {
		impl, err = multiplexed(impl, name, s)
		if err != nil {
			return nil, err
		}
	} else if allowPreconnecting && s.MaxPreconnect > 0 {
		log.Debugf("Enabling preconnecting for %v", name)
		// give ourselves a large margin for making sure we're not using idled preconnected connections
		expiration := IdleTimeout / 2
		impl = newPreconnectingDialer(name, int(s.MaxPreconnect), expiration, impl)
	}

	return impl, err
}
