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
	"sync"
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
	readyChan      chan error
}

var httpClient *http.Client

// waterLoadingWASMMutex prevents the WATER implementation to download/load the
// WASM file concurrently.
var waterTransportLocker sync.Map

func newWaterImpl(dir, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	ctx := context.Background()
	wasmAvailableAt := ptSetting(pc, "water_available_at")
	transport := ptSetting(pc, "water_transport")
	d := &waterImpl{
		raddr:          addr,
		reportDialCore: reportDialCore,
		readyChan:      make(chan error),
	}

	b64WASM := ptSetting(pc, "water_wasm")
	if b64WASM != "" {
		go func() {
			wasm, err := base64.StdEncoding.DecodeString(b64WASM)
			if err != nil {
				d.readyChan <- log.Errorf("failed to decode water wasm: %w", err)
				return
			}

			d.dialer, err = createDialer(ctx, wasm, transport)
			if err != nil {
				d.readyChan <- log.Errorf("failed to create dialer: %w", err)
				return
			}
			d.readyChan <- nil
		}()
		return d, nil
	}

	if wasmAvailableAt != "" {
		go func() {
			log.Debugf("Loading WASM for %q. If not available, it should try to download from the following URLs: %+v. The file should be available at: %q", transport, strings.Split(wasmAvailableAt, ","), dir)

			r, err := d.loadWASM(ctx, transport, dir, wasmAvailableAt)
			if err != nil {
				d.readyChan <- log.Errorf("failed to read wasm: %w", err)
				return
			}
			defer r.Close()
			b, err := io.ReadAll(r)
			if err != nil {
				d.readyChan <- log.Errorf("failed to load wasm bytes: %w", err)
				return
			}

			log.Debugf("received wasm with %d bytes", len(b))

			d.dialer, err = createDialer(ctx, b, transport)
			if err != nil {
				d.readyChan <- log.Errorf("failed to create dialer: %w", err)
				return
			}
			d.readyChan <- nil
		}()
	}

	return d, nil
}

func (d *waterImpl) loadWASM(ctx context.Context, transport string, dir string, wasmAvailableAt string) (io.ReadCloser, error) {
	locker, ok := waterTransportLocker.Load(transport)
	if !ok {
		waterTransportLocker.Store(transport, new(sync.Mutex))
		locker, _ = waterTransportLocker.Load(transport)
	}

	locker.(sync.Locker).Lock()
	defer locker.(sync.Locker).Unlock()
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
	return r, nil
}

func createDialer(ctx context.Context, wasm []byte, transport string) (water.Dialer, error) {
	cfg := &water.Config{
		TransportModuleBin: wasm,
		OverrideLogger:     slog.New(newLogHandler(log, transport)),
	}

	dialer, err := water.NewDialerWithContext(ctx, cfg)
	if err != nil {
		return nil, log.Errorf("failed to create dialer: %w", err)
	}
	return dialer, nil
}

func (d *waterImpl) close() {
	close(d.readyChan)
}

func (d *waterImpl) ready() <-chan error {
	return d.readyChan
}

func (d *waterImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return d.reportDialCore(op, func() (net.Conn, error) {
		// if dialer is not ready, wait until WASM is downloaded or context timeout
		select {
		case err, ok := <-d.readyChan:
			if !ok {
				return nil, log.Error("dialer closed")
			}
			if err != nil {
				return nil, log.Errorf("failed to load dialer: %w", err)
			}

			log.Debug("dialer ready!")
		case <-ctx.Done():
			return nil, log.Errorf("context completed while waiting for WASM download: %w", ctx.Err())
		}

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
