package chained

import (
	"context"
	"fmt"
	"net"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/base"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"
	"github.com/getlantern/flashlight/ops"
)

type obfs4Impl struct {
	nopCloser
	dialCore coreDialer
	addr     string
	cf       base.ClientFactory
	args     interface{}
}

func newOBFS4Impl(name, addr string, s *ChainedServerInfo, dialCore coreDialer) (proxyImpl, error) {
	if s.Cert == "" {
		return nil, fmt.Errorf("No Cert configured for obfs4 server, can't connect")
	}

	cf, err := (&obfs4.Transport{}).ClientFactory("")
	if err != nil {
		return nil, log.Errorf("Unable to create obfs4 client factory: %v", err)
	}

	ptArgs := &pt.Args{}
	ptArgs.Add("cert", s.Cert)
	ptArgs.Add("iat-mode", s.ptSetting("iat-mode"))

	args, err := cf.ParseArgs(ptArgs)
	if err != nil {
		return nil, log.Errorf("Unable to parse client args: %v", err)
	}

	return &obfs4Impl{
		dialCore: dialCore,
		addr:     addr,
		cf:       cf,
		args:     args,
	}, nil
}

func (impl *obfs4Impl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	dial := func(network, address string) (net.Conn, error) {
		// We know for sure the network and address are the same as what
		// the inner DailServer uses.
		return impl.dialCore(op, ctx, impl.addr)
	}
	// The proxy it wrapped already has timeout applied.
	return impl.cf.Dial("whatever", "whatever", dial, impl.args)
}
