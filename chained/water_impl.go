package chained

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/refraction-networking/water"
	_ "github.com/refraction-networking/water/transport/v1"
)

type waterImpl struct {
	raddr          string
	reportDialCore reportDialCoreFn
	dialer         water.Dialer
	nopCloser
}

var httpClient *http.Client

func newWaterImpl(dir, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	var wasm []byte

	b64WASM := ptSetting(pc, "water_wasm")
	if b64WASM != "" {
		var err error
		wasm, err = base64.StdEncoding.DecodeString(b64WASM)
		if err != nil {
			return nil, fmt.Errorf("failed to decode water wasm: %w", err)
		}
	}

	ctx := context.Background()
	wasmAvailableAt := ptSetting(pc, "water_available_at")
	transport := ptSetting(pc, "water_transport")
	if wasm == nil && wasmAvailableAt != "" {
		vc := newWaterVersionControl(dir)
		cli := httpClient
		if cli == nil {
			cli = proxied.ChainedThenDirectThenFrontedClient(1*time.Minute, "")
		}
		downloader, err := newWaterWASMDownloader(strings.Split(wasmAvailableAt, ","), cli)
		if err != nil {
			return nil, log.Errorf("failed to create wasm downloader: %w", err)
		}

		r, err := vc.GetWASM(ctx, transport, downloader)
		if err != nil {
			return nil, log.Errorf("failed to get wasm: %w", err)
		}
		defer r.Close()

		b, err := io.ReadAll(r)
		if err != nil {
			return nil, log.Errorf("failed to read wasm: %w", err)
		}

		if len(b) == 0 {
			return nil, log.Errorf("received empty wasm")
		}

		wasm = b
	}

	cfg := &water.Config{
		TransportModuleBin: wasm,
		OverrideLogger:     slog.New(newLogHandler(log, transport)),
	}

	dialer, err := water.NewDialerWithContext(ctx, cfg)
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
