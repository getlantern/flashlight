package kcpwrapper

import (
	"context"
	"net"
	"time"

	"github.com/getlantern/cmux"
	"github.com/getlantern/errors"
	"github.com/getlantern/kcp-go/v5"
)

// DialerConfig configures dialing
type DialerConfig struct {
	CommonConfig `mapstructure:",squash"`
	Conn         int `json:"conn"`
	AutoExpire   int `json:"autoexpire"`
	ScavengeTTL  int `json:"scavengettl"`
}

// Dialer creates a new dialer function
func Dialer(cfg *DialerConfig, onConn func(net.Conn) net.Conn) func(ctx context.Context, addr string) (net.Conn, error) {
	cfg.applyDefaults()

	log.Debugf("conn: %v", cfg.Conn)
	log.Debugf("autoexpire: %v", cfg.AutoExpire)
	log.Debugf("scavengettl: %v", cfg.ScavengeTTL)

	if onConn == nil {
		onConn = func(conn net.Conn) net.Conn { return conn }
	}

	dialKCP := func(ctx context.Context, _, addr string) (net.Conn, error) {
		kcpconn, err := kcp.DialWithOptions(addr, cfg.block, cfg.DataShard, cfg.ParityShard)
		if err != nil {
			return nil, errors.New("Unable to dial KCP: %v", err)
		}

		kcpconn.SetStreamMode(true)
		kcpconn.SetWriteDelay(true)
		kcpconn.SetNoDelay(cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion)
		kcpconn.SetWindowSize(cfg.SndWnd, cfg.RcvWnd)
		kcpconn.SetMtu(cfg.MTU)
		kcpconn.SetACKNoDelay(cfg.AckNodelay)

		if err := kcpconn.SetDSCP(cfg.DSCP); err != nil {
			log.Debugf("Unable to set DSCP: %v", err)
		}
		if err := kcpconn.SetReadBuffer(cfg.SockBuf); err != nil {
			log.Debugf("Unable to set ReadBuffer: %v", err)
		}
		if err := kcpconn.SetWriteBuffer(cfg.SockBuf); err != nil {
			log.Debugf("Unable to set WriteBuffer:: %v", err)
		}

		log.Debugf("Connected with KCP: %v -> %v", kcpconn.LocalAddr(), kcpconn.RemoteAddr())
		if cfg.NoComp {
			return kcpconn, nil
		}
		return wrapSnappy(onConn(kcpconn)), nil
	}

	dialer := cmux.Dialer(&cmux.DialerOpts{
		Dial:              dialKCP,
		PoolSize:          cfg.Conn,
		BufferSize:        cfg.SockBuf,
		KeepAliveInterval: time.Duration(cfg.KeepAlive) * time.Second,
	})

	return func(ctx context.Context, addr string) (net.Conn, error) {
		return dialer(ctx, "unused", addr)
	}
}
