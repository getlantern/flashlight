package chained

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
)

//go:generate mockgen -package=chained -destination=mocks_test.go . WASMDownloader,TorrentClient,TorrentInfo
//go:generate mockgen -package=chained -destination=torrent_reader_mock_test.go github.com/anacrolix/torrent Reader

type WASMDownloader interface {
	DownloadWASM(context.Context, io.Writer) error
	Close() error
}

type downloader struct {
	urls             []string
	httpClient       *http.Client
	httpDownloader   WASMDownloader
	magnetDownloader WASMDownloader
}

// NewWASMDownloader creates a new WASMDownloader instance.
func NewWASMDownloader(urls []string, httpClient *http.Client) (WASMDownloader, error) {
	if len(urls) == 0 {
		return nil, log.Error("WASM downloader requires URLs to download but received empty list")
	}
	return &downloader{
		urls:       urls,
		httpClient: httpClient,
	}, nil
}

func (d *downloader) Close() error {
	if d.magnetDownloader != nil {
		return d.magnetDownloader.Close()
	}
	return nil
}

// DownloadWASM downloads the WASM file from the given URLs, verifies the hash
// sum and writes the file to the given writer.
func (d *downloader) DownloadWASM(ctx context.Context, w io.Writer) error {
	joinedErrs := errors.New("failed to download WASM from all URLs")
	for _, url := range d.urls {
		tempBuffer := &bytes.Buffer{}
		err := d.downloadWASM(ctx, tempBuffer, url)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
			continue
		}

		_, err = tempBuffer.WriteTo(w)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
			continue
		}

		return nil
	}
	return joinedErrs
}

// downloadWASM checks what kind of URL was given and downloads the WASM file
// from the URL. It can be a HTTPS URL or a magnet link.
func (d *downloader) downloadWASM(ctx context.Context, w io.Writer, url string) error {
	switch {
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"):
		if d.httpDownloader == nil {
			d.httpDownloader = NewHTTPSDownloader(d.httpClient, url)
		}
		return d.httpDownloader.DownloadWASM(ctx, w)
	case strings.HasPrefix(url, "magnet:?"):
		if d.magnetDownloader == nil {
			var err error
			downloader, err := NewMagnetDownloader(ctx, url)
			if err != nil {
				return err
			}
			d.magnetDownloader = downloader
		}
		return d.magnetDownloader.DownloadWASM(ctx, w)
	default:
		return log.Errorf("unsupported protocol: %s", url)
	}
}
