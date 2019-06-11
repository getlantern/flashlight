// +build !android, !linux

package chained

import (
	"context"
	utp "github.com/anacrolix/go-libutp"
	"net"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/mtime"
)

func enableUTP(p *proxy, s *ChainedServerInfo) error {
	socket, err := utp.NewSocket("udp", ":0")
	if err != nil {
		return errors.New("Unable to create utp socket: %v", err)
	}

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()

		conn, err := socket.DialContext(ctx, "udp", p.addr)
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}
