package chained

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestDownloaderWithOptions(t *testing.T) {
	var tests = []struct {
		name         string
		givenOptions []DownloaderOption
		assert       func(*testing.T, *downloader)
	}{
		{
			name: "no options",
			assert: func(t *testing.T, d *downloader) {
				assert.Empty(t, d.urls)
				assert.Nil(t, d.httpClient)
				assert.Empty(t, d.expectedHashSum)
			},
		},
		{
			name: "with URLs",
			givenOptions: []DownloaderOption{
				WithURLs([]string{"http://example.com"}),
			},
			assert: func(t *testing.T, d *downloader) {
				assert.Equal(t, []string{"http://example.com"}, d.urls)
				assert.Nil(t, d.httpClient)
				assert.Empty(t, d.expectedHashSum)
			},
		},
		{
			name: "with HTTP client",
			givenOptions: []DownloaderOption{
				WithHTTPClient(http.DefaultClient),
			},
			assert: func(t *testing.T, d *downloader) {
				assert.Empty(t, d.urls)
				assert.Equal(t, http.DefaultClient, d.httpClient)
				assert.Empty(t, d.expectedHashSum)
			},
		},
		{
			name: "with expected hashsum",
			givenOptions: []DownloaderOption{
				WithExpectedHashsum("hashsum"),
			},
			assert: func(t *testing.T, d *downloader) {
				assert.Empty(t, d.urls)
				assert.Nil(t, d.httpClient)
				assert.Equal(t, "hashsum", d.expectedHashSum)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assert(t, NewWASMDownloader(tt.givenOptions...).(*downloader))
		})
	}
}

func TestDownloadWASM(t *testing.T) {
	ctx := context.Background()

	contentMessage := "content"
	hashsum := sha256.Sum256([]byte(contentMessage))
	contentHashsum := fmt.Sprintf("%x", hashsum[:])
	var tests = []struct {
		name                 string
		givenHTTPClient      *http.Client
		givenURLs            []string
		givenExpectedHashSum string
		givenWriter          io.Writer
		setupHTTPDownloader  func(ctrl *gomock.Controller) WASMDownloader
		assert               func(*testing.T, io.Reader, error)
	}{
		{
			name: "no URLs",
			assert: func(t *testing.T, r io.Reader, err error) {
				b, berr := io.ReadAll(r)
				require.NoError(t, berr)
				assert.Empty(t, b)
				assert.Error(t, err)
				assert.ErrorContains(t, err, "failed to download WASM from all URLs")
			},
		},
		{
			name: "udp urls are unsupported",
			givenURLs: []string{
				"udp://example.com",
			},
			assert: func(t *testing.T, r io.Reader, err error) {
				b, berr := io.ReadAll(r)
				require.NoError(t, berr)
				assert.Empty(t, b)
				assert.Error(t, err)
				assert.ErrorContains(t, err, "unsupported protocol")
			},
		},
		{
			name: "http download error",
			givenURLs: []string{
				"http://example.com",
			},
			setupHTTPDownloader: func(ctrl *gomock.Controller) WASMDownloader {
				httpDownloader := NewMockWASMDownloader(ctrl)
				httpDownloader.EXPECT().DownloadWASM(ctx, gomock.Any()).Return(assert.AnError)
				return httpDownloader
			},
			assert: func(t *testing.T, r io.Reader, err error) {
				b, berr := io.ReadAll(r)
				require.NoError(t, berr)
				assert.Empty(t, b)
				assert.Error(t, err)
				assert.ErrorContains(t, err, assert.AnError.Error())
				assert.ErrorContains(t, err, "failed to download WASM from all URLs")
			},
		},
		{
			name: "hashsum verification error",
			givenURLs: []string{
				"http://example.com",
			},
			givenExpectedHashSum: "hashsum",
			setupHTTPDownloader: func(ctrl *gomock.Controller) WASMDownloader {
				httpDownloader := NewMockWASMDownloader(ctrl)
				httpDownloader.EXPECT().DownloadWASM(ctx, gomock.Any()).DoAndReturn(
					func(ctx context.Context, w io.Writer) error {
						_, err := w.Write([]byte(contentMessage))
						return err
					})
				return httpDownloader
			},
			assert: func(t *testing.T, r io.Reader, err error) {
				b, berr := io.ReadAll(r)
				require.NoError(t, berr)
				assert.Empty(t, b)
				assert.Error(t, err)
				assert.ErrorContains(t, err, "failed to verify hash sum")
			},
		},
		{
			name: "success",
			givenURLs: []string{
				"http://example.com",
			},
			givenExpectedHashSum: contentHashsum,
			setupHTTPDownloader: func(ctrl *gomock.Controller) WASMDownloader {
				httpDownloader := NewMockWASMDownloader(ctrl)
				httpDownloader.EXPECT().DownloadWASM(ctx, gomock.Any()).DoAndReturn(
					func(ctx context.Context, w io.Writer) error {
						_, err := w.Write([]byte(contentMessage))
						return err
					})
				return httpDownloader
			},
			assert: func(t *testing.T, r io.Reader, err error) {
				b, berr := io.ReadAll(r)
				require.NoError(t, berr)
				assert.NoError(t, err)
				assert.Equal(t, contentMessage, string(b))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var httpDownloader WASMDownloader
			if tt.setupHTTPDownloader != nil {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				httpDownloader = tt.setupHTTPDownloader(ctrl)
			}

			b := &bytes.Buffer{}
			err := NewWASMDownloader(
				WithExpectedHashsum(tt.givenExpectedHashSum),
				WithHTTPClient(tt.givenHTTPClient),
				WithURLs(tt.givenURLs),
				WithHTTPDownloader(httpDownloader)).
				DownloadWASM(ctx, b)
			tt.assert(t, b, err)
		})
	}
}
