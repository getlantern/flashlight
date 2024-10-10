package chained

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/anacrolix/torrent"
)

type magnetDownloader struct {
	magnetURL string
}

// NewMagnetDownloader creates a new WASMDownloader instance.
func NewMagnetDownloader(magnetURL string) WASMDownloader {
	return &magnetDownloader{
		magnetURL: magnetURL,
	}
}

// DownloadWASM downloads the WASM file from the given URL.
func (d *magnetDownloader) DownloadWASM(ctx context.Context, w io.Writer) error {
	cfg := torrent.NewDefaultClientConfig()
	path, err := os.MkdirTemp("", "lantern-water-module")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	cfg.DataDir = path
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create torrent client: %w", err)
	}
	defer client.Close()

	t, err := client.AddMagnet(d.magnetURL)
	if err != nil {
		return fmt.Errorf("failed to add magnet: %w", err)
	}
	<-t.GotInfo()
	t.DownloadAll()
	client.WaitAll()

	fmt.Println(t.InfoHash().String())

	_, err = io.Copy(w, t.NewReader())
	if err != nil {
		return fmt.Errorf("failed to copy torrent reader to writer: %w", err)
	}
	return nil
}
