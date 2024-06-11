package chained

import (
	"context"
	"net"
	"os"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
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
	wasmPath := ptSetting(pc, "water_wasm_path")
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, errors.New("failed to read wasm file: %v", err)
	}

	return &waterImpl{
		raddr: addr,
		config: &water.Config{
			TransportModuleBin: wasm,
		},
		reportDialCore: reportDialCore,
	}, nil
}

func (d *waterImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	dialer, err := water.NewDialerWithContext(ctx, d.config)
	if err != nil {
		return nil, errors.New("failed to create dialer: %v", err)
	}

	return d.reportDialCore(op, func() (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", d.raddr)
	})
}
