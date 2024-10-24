package chained

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestNewWASMDownloader(t *testing.T) {
	var tests = []struct {
		name            string
		givenURLs       []string
		givenHTTPClient *http.Client
		assert          func(*testing.T, waterWASMDownloader, error)
	}{
		{
			name: "it should return an error when providing an empty list of URLs",
			assert: func(t *testing.T, d waterWASMDownloader, err error) {
				assert.Error(t, err)
				assert.Nil(t, d)
			},
		},
		{
			name:            "it should successfully return a wasm downloader",
			givenURLs:       []string{"http://example.com"},
			givenHTTPClient: http.DefaultClient,
			assert: func(t *testing.T, wDownloader waterWASMDownloader, err error) {
				assert.NoError(t, err)
				d := wDownloader.(*downloader)
				assert.Equal(t, []string{"http://example.com"}, d.urls)
				assert.Equal(t, http.DefaultClient, d.httpClient)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := newWaterWASMDownloader(tt.givenURLs, tt.givenHTTPClient)
			tt.assert(t, d, err)
		})
	}
}

func TestDownloadWASM(t *testing.T) {
	ctx := context.Background()

	contentMessage := "content"
	var tests = []struct {
		name                string
		givenHTTPClient     *http.Client
		givenURLs           []string
		givenWriter         io.Writer
		setupHTTPDownloader func(ctrl *gomock.Controller) waterWASMDownloader
		assert              func(*testing.T, io.Reader, error)
	}{
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
			setupHTTPDownloader: func(ctrl *gomock.Controller) waterWASMDownloader {
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
			name: "success",
			givenURLs: []string{
				"http://example.com",
			},
			setupHTTPDownloader: func(ctrl *gomock.Controller) waterWASMDownloader {
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
			var httpDownloader waterWASMDownloader
			if tt.setupHTTPDownloader != nil {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				httpDownloader = tt.setupHTTPDownloader(ctrl)
			}

			b := &bytes.Buffer{}
			d, err := newWaterWASMDownloader(tt.givenURLs, tt.givenHTTPClient)
			require.NoError(t, err)

			if httpDownloader != nil {
				d.(*downloader).httpDownloader = httpDownloader
			}
			err = d.DownloadWASM(ctx, b)
			tt.assert(t, b, err)
		})
	}
}
