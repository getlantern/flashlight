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
	raddr            string
	reportDialCore   reportDialCoreFn
	dialer           water.Dialer
	errLoadingWASM   error
	ready            bool
	loadingWASMMutex sync.Locker
	finishedToLoad   chan struct{}
}

var httpClient *http.Client

func newWaterImpl(dir, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	ctx := context.Background()
	wasmAvailableAt := ptSetting(pc, "water_available_at")
	transport := ptSetting(pc, "water_transport")
	d := &waterImpl{
		raddr:            addr,
		reportDialCore:   reportDialCore,
		loadingWASMMutex: new(sync.Mutex),
		finishedToLoad:   make(chan struct{}),
	}

	b64WASM := ptSetting(pc, "water_wasm")
	if b64WASM != "" {
		go func() {
			defer d.finishedLoading()
			wasm, err := base64.StdEncoding.DecodeString(b64WASM)
			if err != nil {
				d.errLoadingWASM = log.Errorf("failed to decode water wasm: %w", err)
				return
			}

			d.dialer, err = createDialer(ctx, wasm, transport)
			if err != nil {
				d.errLoadingWASM = log.Errorf("failed to create dialer: %w", err)
				return
			}
			d.setReady()
		}()
		return d, nil
	}

	if wasmAvailableAt != "" {
		go func() {
			defer d.finishedLoading()
			log.Debugf("Loading WASM for %q. If not available, it should try to download from the following URLs: %+v. The file should be available here: %s", transport, strings.Split(wasmAvailableAt, ","), dir)
			vc := newWaterVersionControl(dir)
			cli := httpClient
			if cli == nil {
				cli = proxied.ChainedThenDirectThenFrontedClient(1*time.Minute, "")
			}
			downloader, err := newWaterWASMDownloader(strings.Split(wasmAvailableAt, ","), cli)
			if err != nil {
				d.errLoadingWASM = log.Errorf("failed to create wasm downloader: %w", err)
				return
			}

			r, err := vc.GetWASM(ctx, transport, downloader)
			if err != nil {
				d.errLoadingWASM = log.Errorf("failed to get wasm: %w", err)
				return
			}
			defer r.Close()

			b, err := io.ReadAll(r)
			if err != nil {
				d.errLoadingWASM = log.Errorf("failed to read wasm: %w", err)
				return
			}

			if len(b) == 0 {
				d.errLoadingWASM = log.Errorf("received empty wasm")
				return
			}

			d.dialer, err = createDialer(ctx, b, transport)
			if err != nil {
				d.errLoadingWASM = log.Errorf("failed to create dialer: %w", err)
				return
			}
			d.setReady()
		}()
	}

	return d, nil
}

func (d *waterImpl) finishedLoading() {
	select {
	case d.finishedToLoad <- struct{}{}:
	default:
		log.Error("Channel is closed")
	}
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

func (d *waterImpl) isReady() (bool, error) {
	d.loadingWASMMutex.Lock()
	defer d.loadingWASMMutex.Unlock()
	return d.ready, d.errLoadingWASM
}

func (d *waterImpl) setReady() {
	d.loadingWASMMutex.Lock()
	defer d.loadingWASMMutex.Unlock()
	d.ready = true
}

func (d *waterImpl) close() {
	close(d.finishedToLoad)
}

func (d *waterImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return d.reportDialCore(op, func() (net.Conn, error) {
		// if dialer is not ready, wait until WASM is downloaded or context timeout
		ready, err := d.isReady()
		if err != nil {
			return nil, log.Errorf("failed to load WASM for dialer: %w", err)
		}

		if !ready {
			select {
			case _, ok := <-d.finishedToLoad:
				if !ok {
					d.errLoadingWASM = log.Error("Channel closed")
					return nil, d.errLoadingWASM
				}
				log.Debug("download finished!")
			case <-ctx.Done():
				d.errLoadingWASM = log.Errorf("context completed while waiting for WASM download: %w", ctx.Err())
				return nil, d.errLoadingWASM
			}
		}

		if d.dialer == nil || d.errLoadingWASM != nil {
			return nil, log.Errorf("dialer not available: %w", d.errLoadingWASM)
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
