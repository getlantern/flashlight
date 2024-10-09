package chained

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

//go:generate mockgen -destination=mocks.go -package=chained . WASMDownloader

type WASMDownloader interface {
	DownloadWASM(context.Context, io.Writer) error
}

type downloader struct {
	urls            []string
	httpClient      *http.Client
	expectedHashSum string
	httpDownloader  WASMDownloader
}

type DownloaderOption func(*downloader)

func WithURLs(urls []string) DownloaderOption {
	return func(d *downloader) {
		d.urls = urls
	}
}

func WithHTTPClient(httpClient *http.Client) DownloaderOption {
	return func(d *downloader) {
		d.httpClient = httpClient
	}
}

func WithExpectedHashsum(hashsum string) DownloaderOption {
	return func(d *downloader) {
		d.expectedHashSum = hashsum
	}
}

func WithHTTPDownloader(httpDownloader WASMDownloader) DownloaderOption {
	return func(d *downloader) {
		d.httpDownloader = httpDownloader
	}
}

// NewWASMDownloader creates a new WASMDownloader instance.
func NewWASMDownloader(withOpts ...DownloaderOption) WASMDownloader {
	downloader := new(downloader)
	for _, opt := range withOpts {
		opt(downloader)
	}
	return downloader
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

		err = d.verifyHashSum(tempBuffer.Bytes())
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
		return NewMagnetDownloader(url).DownloadWASM(ctx, w)
	default:
		return fmt.Errorf("unsupported protocol: %s", url)
	}
}

var ErrFailedToVerifyHashSum = errors.New("failed to verify hash sum")

func (d *downloader) verifyHashSum(data []byte) error {
	sha256Hashsum := sha256.Sum256(data)
	if d.expectedHashSum == "" || d.expectedHashSum != fmt.Sprintf("%x", sha256Hashsum[:]) {
		return ErrFailedToVerifyHashSum
	}
	return nil
}
