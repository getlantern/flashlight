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
	_ "github.com/refraction-networking/water/transport/v0"
)

type waterImpl struct {
	config         *water.Config
	raddr          string
	reportDialCore reportDialCoreFn
	nopCloser
}

func newWaterImpl(addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	b64WASM := ptSetting(pc, "water_wasm")
	wasm, err := base64.StdEncoding.DecodeString(b64WASM)
	if err != nil {
		return nil, fmt.Errorf("failed to decode water wasm: %w", err)
	}

	return &waterImpl{
		raddr: addr,
		config: &water.Config{
			TransportModuleBin: wasm,
			OverrideLogger:     slog.New(newLogHandler(log, ptSetting(pc, "water_transport"))),
		},
		reportDialCore: reportDialCore,
	}, nil
}

func (d *waterImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	dialer, err := water.NewDialerWithContext(ctx, d.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialer: %w", err)
	}

	return d.reportDialCore(op, func() (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", d.raddr)
	})
}
