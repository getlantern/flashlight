package chained

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/tls"
	"github.com/Jigsaw-Code/outline-sdk/transport/tlsfrag"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/ops"
)

type tlsfragImpl struct {
	nopCloser
	addr   string
	dialer transport.StreamDialer
}

var _ proxyImpl = (*tlsfragImpl)(nil)

func newTLSFrag(addr string, proxyConfig *config.ProxyConfig) (proxyImpl, error) {
	lenStr, ok := proxyConfig.PluggableTransportSettings["splitLen"]
	if !ok {
		return nil, fmt.Errorf("splitLen option is missing")
	}

	fixedLen, err := strconv.Atoi(lenStr)
	if err != nil {
		return nil, fmt.Errorf("invalid tlsfrag option: %v. It should be a number", lenStr)
	}

	tlsDialer, err := tls.NewStreamDialer(&transport.TCPStreamDialer{})
	if err != nil {
		return nil, fmt.Errorf("failed to create tls dialer: %v", err)
	}

	dialer, err := tlsfrag.NewFixedLenStreamDialer(tlsDialer, fixedLen)
	if err != nil {
		return nil, fmt.Errorf("failed to create tlsfrag dialer: %v", err)
	}
	return &tlsfragImpl{addr: addr, dialer: dialer}, nil
}

func (impl *tlsfragImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.dialer.Dial(ctx, impl.addr)
}
