package chained

import (
	"context"
	"errors"
	"io"
	"net"
	"os"

	"github.com/anacrolix/chansync/events"
	"github.com/anacrolix/torrent"
)

type waterMagnetDownloader struct {
	magnetURL string
	client    torrentClient
}

// newWaterMagnetDownloader creates a new WASMDownloader instance.
func newWaterMagnetDownloader(ctx context.Context, magnetURL string) (waterWASMDownloader, error) {
	cfg, err := generateTorrentClientConfig(ctx)
	if err != nil {
		return nil, err
	}

	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, log.Errorf("failed to create torrent client: %w", err)
	}
	return &waterMagnetDownloader{
		magnetURL: magnetURL,
		client:    newTorrentCliWrapper(client),
	}, nil
}

func (d *waterMagnetDownloader) Close() error {
	errs := d.client.Close()
	closeErr := errors.New("failed to close torrent client")
	allErrs := make([]error, len(errs)+1)
	allErrs[0] = closeErr
	for i, err := range errs {
		allErrs[i+1] = err
	}
	closeErr = errors.Join(allErrs...)
	return closeErr
}

type torrentCliWrapper struct {
	client *torrent.Client
}

func newTorrentCliWrapper(client *torrent.Client) *torrentCliWrapper {
	return &torrentCliWrapper{
		client: client,
	}
}

func (t *torrentCliWrapper) AddMagnet(magnetURL string) (torrentInfo, error) {
	return t.client.AddMagnet(magnetURL)
}

func (t *torrentCliWrapper) Close() []error {
	return t.client.Close()
}

type torrentClient interface {
	AddMagnet(string) (torrentInfo, error)
	Close() []error
}

type torrentInfo interface {
	GotInfo() events.Done
	NewReader() torrent.Reader
}

func dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	select {
	case <-ctx.Done():
		return nil, log.Errorf("context complete: %w", ctx.Err())
	default:
		return new(net.Dialer).DialContext(ctx, network, addr)
	}
}

func generateTorrentClientConfig(ctx context.Context) (*torrent.ClientConfig, error) {
	cfg := torrent.NewDefaultClientConfig()
	path, err := os.MkdirTemp("", "lantern-water-module")
	if err != nil {
		return nil, log.Errorf("failed to create temp dir: %w", err)
	}
	cfg.DataDir = path
	cfg.HTTPDialContext = dialContext
	cfg.TrackerDialContext = dialContext
	return cfg, nil
}

// DownloadWASM downloads the WASM file from the given URL.
func (d *waterMagnetDownloader) DownloadWASM(ctx context.Context, w io.Writer) error {
	t, err := d.client.AddMagnet(d.magnetURL)
	if err != nil {
		return log.Errorf("failed to add magnet: %w", err)
	}

	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		return log.Errorf("context complete: %w", ctx.Err())
	}

	_, err = io.Copy(w, t.NewReader())
	if err != nil {
		return log.Errorf("failed to copy torrent reader to writer: %w", err)
	}
	return nil
}
