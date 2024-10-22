package chained

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
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

func newWaterImpl(addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	var wasm []byte

	b64WASM := ptSetting(pc, "water_wasm")
	if b64WASM != "" {
		var err error
		wasm, err = base64.StdEncoding.DecodeString(b64WASM)
		if err != nil {
			return nil, fmt.Errorf("failed to decode water wasm: %w", err)
		}
	}

	wasmAvailableAt := ptSetting(pc, "water_available_at")
	if wasm == nil && wasmAvailableAt != "" {
		urls := strings.Split(wasmAvailableAt, ",")
		b := new(bytes.Buffer)
		cli := httpClient
		if cli == nil {
			downloadTimeout := ptSetting(pc, "water_download_timeout")
			timeout := 10 * time.Minute
			if downloadTimeout != "" {
				var err error
				timeout, err = time.ParseDuration(downloadTimeout)
				if err != nil {
					return nil, fmt.Errorf("failed to parse download timeout: %w", err)
				}
			}

			cli = proxied.ChainedThenDirectThenFrontedClient(timeout, "")
		}
		d, err := NewWASMDownloader(urls, cli)
		if err != nil {
			return nil, log.Errorf("failed to create wasm downloader: %s", err.Error())
		}
		if err = d.DownloadWASM(context.Background(), b); err != nil {
			return nil, log.Errorf("failed to download wasm: %s", err.Error())
		}
		wasm = b.Bytes()
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
