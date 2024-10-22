package chained

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type httpsDownloader struct {
	cli *http.Client
	url string
}

func NewHTTPSDownloader(client *http.Client, url string) WASMDownloader {
	return &httpsDownloader{cli: client, url: url}
}

func (d *httpsDownloader) Close() error {
	return nil
}

func (d *httpsDownloader) DownloadWASM(ctx context.Context, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create a new HTTP request: %w", err)
	}
	resp, err := d.cli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send a HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download WASM file: %s", resp.Status)
	}

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write the WASM file: %w", err)
	}
	return nil
}
