// +build android iosapp linux

package chained

import (
	"context"
	"errors"
	"net"
)

func utpDialer() (func(ctx context.Context, addr string) (net.Conn, error), error) {
	return nil, errors.New("UTP is not supported on Android, iOS or Linux")
}
