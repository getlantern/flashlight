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
	// readyChannels is a list of channels that are waiting for the dialer to be ready.
	readyChannels []chan error
	// readyChanLock protects access/modifications to readyChannels, finishedLoading flag and errLoadingWASM.
	readyChanLock   sync.Locker
	finishedLoading bool
	errLoadingWASM  error
	nopCloser
}

var (
	// waterHTTPClient is a shared HTTP client for downloading WebAssembly modules.
	waterHTTPClient *http.Client

	// waterWASMLocks is a map of mutexes protecting RW access to the WebAssembly module files for each
	// WATER transport. A goroutine must acquire the lock for transport "foo" in order to write the
	// foo.wasm file or read the foo.wasm file on disk.
	//
	// Invariant: entries to this map are never overwritten.
	waterWASMLocks = make(map[string]*sync.Mutex)
	// wlLock protects access to the waterWASMLocks map.
	wlLock = new(sync.Mutex)
)

func newWaterImpl(dir, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*waterImpl, error) {
	ctx := context.Background()
	wasmAvailableAt := ptSetting(pc, "water_available_at")
	transport := ptSetting(pc, "water_transport")
	d := &waterImpl{
		raddr:          addr,
		reportDialCore: reportDialCore,
		readyChannels:  make([]chan error, 0),
		readyChanLock:  new(sync.Mutex),
	}

	b64WASM := ptSetting(pc, "water_wasm")
	if b64WASM != "" {
		go func() {
			wasm, err := base64.StdEncoding.DecodeString(b64WASM)
			if err != nil {
				d.setErrLoadingWASM(log.Errorf("failed to decode water wasm: %w", err))
				return
			}

			d.dialer, err = createDialer(ctx, wasm, transport)
			if err != nil {
				d.setErrLoadingWASM(log.Errorf("failed to create dialer: %w", err))
				return
			}
			d.setFinishedLoadingSuccessfully()
		}()
		return d, nil
	}

	if wasmAvailableAt != "" {
		go func() {
			log.Debugf("Loading WASM for %q. If not available, it should try to download from the following URLs: %+v. The file should be available at: %q", transport, strings.Split(wasmAvailableAt, ","), dir)

			r, err := d.loadWASM(ctx, transport, dir, wasmAvailableAt)
			if err != nil {
				d.setErrLoadingWASM(log.Errorf("failed to read wasm: %w", err))
				return
			}
			defer r.Close()
			b, err := io.ReadAll(r)
			if err != nil {
				d.setErrLoadingWASM(log.Errorf("failed to load wasm bytes: %w", err))
				return
			}

			log.Debugf("received wasm with %d bytes", len(b))

			d.dialer, err = createDialer(ctx, b, transport)
			if err != nil {
				d.setErrLoadingWASM(log.Errorf("failed to create dialer: %w", err))
				return
			}
			d.setFinishedLoadingSuccessfully()
		}()
	}

	return d, nil
}

// ready returns a channel that will be closed when the dialer is ready to be used.
// If the dialer is already ready, this will return nil.
// If the dialer failed to load, this will return a channel that will return the error received while loading the WASM file and close immediately.
// If the dialer is still loading, this will return a channel that will be closed when the dialer is ready or failed to load.
func (d *waterImpl) ready() <-chan error {
	d.readyChanLock.Lock()
	defer d.readyChanLock.Unlock()

	if d.finishedLoading {
		return nil
	}

	if d.errLoadingWASM != nil {
		tempChan := make(chan error)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			select {
			case tempChan <- d.errLoadingWASM:
			case <-ctx.Done():
			}
			close(tempChan)
		}()
		return tempChan
	}

	readyChan := make(chan error)
	d.readyChannels = append(d.readyChannels, readyChan)
	return readyChan
}

func (d *waterImpl) setFinishedLoadingSuccessfully() {
	d.readyChanLock.Lock()
	defer d.readyChanLock.Unlock()
	d.finishedLoading = true
	d.broadcastReadyState(nil)
}

func (d *waterImpl) setErrLoadingWASM(err error) {
	d.readyChanLock.Lock()
	defer d.readyChanLock.Unlock()
	d.errLoadingWASM = err
	d.broadcastReadyState(d.errLoadingWASM)
}

func (d *waterImpl) broadcastReadyState(err error) {
	wg := new(sync.WaitGroup)
	for _, c := range d.readyChannels {
		wg.Add(1)
		go func(err error) {
			defer wg.Done()
			// if the channel is not being listened to, this will hold until the channel is read
			// or the context times out
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			select {
			case c <- err:
			case <-ctx.Done():
			}
			close(c)
		}(err)
	}
	wg.Wait()
}

func (d *waterImpl) loadWASM(ctx context.Context, transport string, dir string, wasmAvailableAt string) (io.ReadCloser, error) {
	wlLock.Lock()
	m, ok := waterWASMLocks[transport]
	if !ok {
		m = new(sync.Mutex)
		waterWASMLocks[transport] = m
	}
	wlLock.Unlock()

	m.Lock()
	defer m.Unlock()

	vc := newWaterVersionControl(dir)
	cli := waterHTTPClient
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

func (d *waterImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return d.reportDialCore(op, func() (net.Conn, error) {
		// if dialer is not ready, wait until WASM is downloaded or context timeout
		isReady := d.ready()
		if isReady != nil {
			select {
			case err := <-isReady:
				if err != nil {
					return nil, log.Errorf("failed to load dialer: %w", err)
				}

				log.Debug("dialer ready!")
			case <-ctx.Done():
				return nil, log.Errorf("context completed while waiting for WASM download: %w", ctx.Err())
			}
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
