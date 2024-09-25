package chained

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/refraction-networking/water"
	_ "github.com/refraction-networking/water/transport/v1"
)

type waterImpl struct {
	raddr          string
	reportDialCore reportDialCoreFn
	dialer         water.Dialer
	nopCloser
}

func newWaterImpl(addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	b64WASM := ptSetting(pc, "water_wasm")
	wasm, err := base64.StdEncoding.DecodeString(b64WASM)
	if err != nil {
		return nil, fmt.Errorf("failed to decode water wasm: %w", err)
	}

	cfg := &water.Config{
		TransportModuleBin: wasm,
		OverrideLogger:     slog.New(newLogHandler(log, ptSetting(pc, "water_transport"))),
	}

	dialer, err := water.NewDialerWithContext(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialer: %w", err)
	}

	return &waterImpl{
		raddr:          addr,
		dialer:         dialer,
		reportDialCore: reportDialCore,
	}, nil
}

func (d *waterImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return d.reportDialCore(op, func() (net.Conn, error) {
		// TODO: At water 0.7.0 (currently), the library is	hanging onto the dial context
		// beyond it's scope. If you cancel this context, all dialed connections with the context
		// will be closed. This should not happen (only dials in progress should be affected).
		// The refraction-networking team is working on that issue and it can be tracked here:
		// https://github.com/refraction-networking/water/issues/75
		// After the issue is resolved, we can remove the context.Background() and use the passed ctx.
		conn, err := d.dialer.DialContext(context.Background(), "tcp", d.raddr)
		if err != nil {
			log.Errorf("failed to dial with water: %v", err)
			return nil, fmt.Errorf("failed to dial with water: %v", err)
		}

		return conn, nil
	})
}
