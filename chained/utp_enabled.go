// +build !android
// +build !iosapp
// +build !ios
// +build !linux

package chained

import (
	"context"
	"net"

	utp "github.com/anacrolix/go-libutp"
	"github.com/getlantern/errors"
)

func utpDialer() (func(ctx context.Context, addr string) (net.Conn, error), error) {
	socket, err := utp.NewSocket("udp", ":0")
	if err != nil {
		return nil, errors.New("Unable to create utp socket: %v", err)
	}
	return func(ctx context.Context, addr string) (net.Conn, error) {
		return socket.DialContext(ctx, "udp", addr)
	}, nil
}
