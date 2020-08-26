// +build !android
// +build !ios
// +build !linux

package chained

import (
	"context"
	"net"

	utp "github.com/anacrolix/go-libutp"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
)

type utpImpl struct {
	proxyImpl
	addr   string
	socket *utp.Socket
}

func enableUTP(wrapped proxyImpl, addr string) (proxyImpl, error) {
	socket, err := utp.NewSocket("udp", ":0")
	if err != nil {
		return nil, errors.New("Unable to create utp socket: %v", err)
	}
	return &utpImpl{wrapped, addr, socket}, nil
}

func (impl *utpImpl) dialCore(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.socket.DialContext(ctx, "udp", impl.addr)
}
