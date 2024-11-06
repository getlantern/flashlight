package chained

import (
	"context"
	"fmt"
	"net"

	"github.com/xtaci/smux"

	"github.com/getlantern/cmux/v2"
	"github.com/getlantern/cmuxprivate"
	"github.com/getlantern/common/config"
	"github.com/getlantern/psmux"

	"github.com/getlantern/flashlight/v7/ops"
)

const (
	defaultMuxProtocol = "smux"
)

type multiplexedImpl struct {
	proxyImpl
	multiplexedDial cmux.DialFN
}

func multiplexed(wrapped proxyImpl, name string, s *config.ProxyConfig) (proxyImpl, error) {
	log.Debugf("Enabling multiplexing for %v", name)
	poolSize := s.MultiplexedPhysicalConns
	if poolSize < 1 {
		poolSize = defaultMultiplexedPhysicalConns
	}

	proto, err := createMultiplexedProtocol(s)
	if err != nil {
		return nil, err
	}
	multiplexedDial := cmux.Dialer(&cmux.DialerOpts{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			op := ops.Begin("dial_multiplexed")
			defer op.End()
			return wrapped.dialServer(op, ctx)
		},
		PoolSize: int(poolSize),
		Protocol: proto,
	})
	return &multiplexedImpl{wrapped, multiplexedDial}, nil
}

func (impl *multiplexedImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.multiplexedDial(ctx, "", "")
}

func (impl *multiplexedImpl) isReady() bool {
	return true
}

// createMultiplexedProtocol configures a cmux multiplexing protocol
// according to settings.
func createMultiplexedProtocol(s *config.ProxyConfig) (cmux.Protocol, error) {
	protocol := s.MultiplexedProtocol
	if protocol == "" {
		protocol = defaultMuxProtocol
	}

	switch protocol {
	case "smux":
		return configureSmux(s)
	case "psmux":
		return configurePsmux(s)
	default:
		return nil, fmt.Errorf("unsupported multiplexing protocol: %v", protocol)
	}
}

func configureSmux(pc *config.ProxyConfig) (cmux.Protocol, error) {
	config := smux.DefaultConfig()
	config.KeepAliveInterval = IdleTimeout / 2
	config.KeepAliveTimeout = IdleTimeout
	config.KeepAliveDisabled = muxSettingBool(pc, "keepalivedisabled")
	if v := muxSettingInt(pc, "version"); v > 0 {
		config.Version = v
	}
	if v := muxSettingInt(pc, "maxframesize"); v > 0 {
		config.MaxFrameSize = v
	}
	if v := muxSettingInt(pc, "maxreceivebuffer"); v > 0 {
		config.MaxReceiveBuffer = v
	}
	if v := muxSettingInt(pc, "maxstreambuffer"); v > 0 {
		config.MaxStreamBuffer = v
	}
	return cmux.NewSmuxProtocol(config), nil
}

func configurePsmux(pc *config.ProxyConfig) (cmux.Protocol, error) {
	config := psmux.DefaultConfig()
	config.KeepAliveInterval = IdleTimeout / 2
	config.KeepAliveTimeout = IdleTimeout
	config.KeepAliveDisabled = muxSettingBool(pc, "keepalivedisabled")
	if v := muxSettingInt(pc, "version"); v > 0 {
		config.Version = v
	}
	if v := muxSettingInt(pc, "maxframesize"); v > 0 {
		config.MaxFrameSize = v
	}
	if v := muxSettingInt(pc, "maxreceivebuffer"); v > 0 {
		config.MaxReceiveBuffer = v
	}
	if v := muxSettingInt(pc, "maxstreambuffer"); v > 0 {
		config.MaxStreamBuffer = v
	}
	if v := muxSettingFloat(pc, "maxpaddingratio"); v != 0.0 {
		if v < 0 { // explicit disable
			config.MaxPaddingRatio = 0.0
		} else {
			config.MaxPaddingRatio = v
		}
	}
	if v := muxSettingInt(pc, "maxpaddedsize"); v != 0 {
		if v < 0 { // explicit disable
			config.MaxPaddedSize = 0
		} else {
			config.MaxPaddedSize = v
		}
	}
	if v := muxSettingInt(pc, "aggressivepadding"); v != 0 {
		if v < 0 { // explicit disable
			config.AggressivePadding = 0
		} else {
			config.AggressivePadding = v
		}
	}
	if v := muxSettingFloat(pc, "aggressivepaddingratio"); v != 0.0 {
		if v < 0 { // explicit disable
			config.AggressivePaddingRatio = 0.0
		} else {
			config.AggressivePaddingRatio = v
		}
	}

	return cmuxprivate.NewPsmuxProtocol(config), nil
}
