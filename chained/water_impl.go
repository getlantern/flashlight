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
		conn, err := d.dialer.DialContext(context.Background(), "tcp", d.raddr)
		if err != nil {
			log.Errorf("failed to dial with water: %v", err)
			return nil, fmt.Errorf("failed to dial with water: %v", err)
		}

		return conn, nil
	})
}
