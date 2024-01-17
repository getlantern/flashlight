package chained

import (
	"context"
	"net"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/tlsdialer/v3"
)

type httpsImpl struct {
	nopCloser
	dialCore                coreDialer
	addr                    string
	tlsConfig               *tls.Config
	roller                  *helloRoller
	tlsClientHelloSplitting bool
	requiresSessionTickets  bool
	proxyConfig             *config.ProxyConfig
	sync.Mutex
}

func newHTTPSImpl(configDir, name, addr string, pc *config.ProxyConfig, uc common.UserConfig, dialCore coreDialer) (proxyImpl, error) {
	const timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tlsConfig, hellos, err := tlsConfigForProxy(ctx, configDir, name, pc, uc)
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	if len(hellos) == 0 {
		return nil, log.Error(errors.New("expected at least one hello"))
	}

	return &httpsImpl{
		dialCore:                dialCore,
		addr:                    addr,
		tlsConfig:               tlsConfig,
		roller:                  &helloRoller{hellos: hellos},
		tlsClientHelloSplitting: pc.TLSClientHelloSplitting,
		requiresSessionTickets:  pc.TLSClientSessionState != "",
		proxyConfig:             pc,
	}, nil
}

func (impl *httpsImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	r := impl.roller.getCopy()
	defer impl.roller.updateTo(r)

	currentHello := r.current()
	helloID, helloSpec, err := currentHello.utlsSpec()
	if err != nil {
		r.advance()
		return nil, errors.New("failed to generate valid utls hello spec, advancing roller: %v", err)
	}

	if impl.requiresSessionTickets && helloID != tls.HelloGolang {
		supportsSessionTickets, err := currentHello.supportsSessionTickets()
		if err != nil {
			r.advance()
			return nil, errors.New("unable to determine if hello %v supports session tickets, advancing roller: %v", helloID, err)
		}
		if !supportsSessionTickets {
			r.advance()
			return nil, errors.New("session ticket is required, but hello %v does not support them; advancing roller", helloID)
		}
	}

	d := tlsdialer.Dialer{
		DoDial: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			tcpConn, err := impl.dialCore(op, ctx, impl.addr)
			if err != nil {
				return nil, err
			}
			if impl.tlsClientHelloSplitting {
				tcpConn = hellosplitter.Wrap(tcpConn, splitClientHello)
			}
			tcpConn = tlsFragConn(tcpConn, impl.proxyConfig)

			return tcpConn, err
		},
		Timeout:         timeoutFor(ctx),
		SendServerName:  impl.tlsConfig.ServerName != "",
		Config:          impl.tlsConfig.Clone(),
		ClientHelloID:   helloID,
		ClientHelloSpec: helloSpec,
	}
	result, err := d.DialForTimings("tcp", impl.addr)
	if err != nil {
		if isHelloErr(err) {
			log.Debugf("got error likely related to bad hello; advancing roller: %v", err)
			r.advance()
		}
		return nil, err
	}
	return result.Conn, nil
}

func timeoutFor(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		return time.Until(deadline)
	}
	return chainedDialTimeout
}
