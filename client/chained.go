package client

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/netx"
)

var (
	// Close connections idle for a period to avoid dangling connections. 45 seconds
	// is long enough to avoid interrupt normal connections but shorter than the
	// idle timeout on the server to avoid running into closed connection problems.
	// 45 seconds is also longer than the MaxIdleTime on our http.Transport, so it
	// doesn't interfere with that.
	idleTimeout = 45 * time.Second
)

// ChainedDialer creates a *balancer.Dialer backed by a chained server.
func ChainedDialer(name string, si *chained.ChainedServerInfo, deviceID string, proTokenGetter func() string) (*balancer.Dialer, error) {
	s, err := newServer(name, si)
	if err != nil {
		return nil, err
	}
	return s.dialer(deviceID, proTokenGetter)
}

type chainedServer struct {
	chained.Proxy
}

func newServer(name string, _si *chained.ChainedServerInfo) (*chainedServer, error) {
	// Copy server info to allow modifying
	si := &chained.ChainedServerInfo{}
	*si = *_si
	// Backwards-compatibility for clients that still have old obfs4
	// configurations on disk.
	if si.PluggableTransport == "obfs4-tcp" {
		si.PluggableTransport = "obfs4"
	}

	p, err := chained.CreateProxy(name, si)
	if err != nil {
		return nil, err
	}
	return &chainedServer{p}, nil
}

func (s *chainedServer) dialer(deviceID string, proTokenGetter func() string) (*balancer.Dialer, error) {
	ccfg := chained.Config{
		DialServer: s.Proxy.DialServer,
		Label:      s.Label(),
		OnRequest: func(req *http.Request) {
			s.attachHeaders(req, deviceID, proTokenGetter)
		},
		OnFinish: func(op *ops.Op) {
			op.ChainedProxy(s.Addr(), s.Proxy.Protocol(), s.Proxy.Network())
		},
		ShouldResetBBR:    s.ShouldResetBBR,
		OnConnectResponse: s.CollectBBRInfo,
	}
	d := chained.NewDialer(ccfg)
	return &balancer.Dialer{
		Label:   s.Label(),
		Trusted: s.Trusted(),
		DialFN: func(network, addr string) (net.Conn, error) {
			var conn net.Conn
			var err error

			op := ops.Begin("dial_for_balancer").ProxyType(ops.ProxyChained).ProxyAddr(s.Addr())
			defer op.End()

			if addr == s.Addr() {
				// Check if we are trying to connect to our own server and bypass proxying if so
				// This accounts for the case w/ multiple instances of Lantern running on mobile
				// Whenever full-device VPN mode is enabled, we need to make sure we ignore proxy
				// requests from the first instance.
				log.Debugf("Attempted to dial ourselves. Dialing directly to %s instead", addr)
				conn, err = netx.DialTimeout("tcp", addr, 1*time.Minute)
			} else {
				return d(network, addr)
			}

			if err != nil {
				return nil, op.FailIf(err)
			}
			conn = idletiming.Conn(conn, idleTimeout, func() {
				log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), idleTimeout)
			})
			return conn, nil
		},
		EstLatency:   s.Proxy.EstLatency,
		EstBandwidth: s.Proxy.EstBandwidth,
	}, nil
}

func (s *chainedServer) attachHeaders(req *http.Request, deviceID string, proTokenGetter func() string) {
	s.Proxy.AdaptRequest(req)
	req.Header.Set("X-Lantern-Device-Id", deviceID)
	if token := proTokenGetter(); token != "" {
		req.Header.Set("X-Lantern-Pro-Token", token)
	}
}
