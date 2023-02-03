package chained

import (
	"context"
	"net"

	"github.com/OperatorFoundation/Starbridge-go/Starbridge/v3"
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/ops"
)

type starbridge struct {
	reportDialCore reportDialCoreFn
	config         Starbridge.ClientConfig
	conn           net.Conn
}

func (s *starbridge) DialServer(op *ops.Op, ctx context.Context, prefix []byte) (net.Conn, error) {
	return s.reportDialCore(op, func() (net.Conn, error) {
		conn, err := s.config.Dial(s.config.Address)
		if err != nil {
			return nil, err
		}
		s.conn = conn
		return conn, nil
	})
}

func (s *starbridge) Close() {
	if s.conn != nil {
		s.conn.Close()
	}
}

func newStarbridgeImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (ProxyImpl, error) {
	config := Starbridge.ClientConfig{
		Address:                   addr,
		ServerPersistentPublicKey: pc.Cert,
	}
	return &starbridge{
		config:         config,
		reportDialCore: reportDialCore,
	}, nil
}
