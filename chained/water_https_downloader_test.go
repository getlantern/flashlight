package water

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc struct {
	f func(req *http.Request) (*http.Response, error)
}

func (f *roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.f(req)
}

func TestHTTPSDownloadWASM(t *testing.T) {
	ctx := context.Background()
	var tests = []struct {
		name            string
		givenHTTPClient *http.Client
		givenURL        string
		assert          func(*testing.T, io.Reader, error)
	}{
		{
			name: "sending request successfully",
			givenHTTPClient: &http.Client{
				Transport: &roundTripFunc{
					f: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString("wasm")),
						}, nil
					},
				},
			},
			givenURL: "https://example.com/wasm.wasm",
			assert: func(t *testing.T, r io.Reader, err error) {
				assert.NoError(t, err)
				b, err := io.ReadAll(r)
				require.NoError(t, err)
				assert.Equal(t, "wasm", string(b))
			},
		},
		{
			name: "when receiving an error from the HTTP client, it should return an error",
			givenHTTPClient: &http.Client{
				Transport: &roundTripFunc{
					f: func(req *http.Request) (*http.Response, error) {
						return nil, assert.AnError
					},
				},
			},
			givenURL: "https://example.com/wasm.wasm",
			assert: func(t *testing.T, r io.Reader, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to send a HTTP request")
			},
		},
		{
			name: "when the HTTP status code is not 200, it should return an error",
			givenHTTPClient: &http.Client{
				Transport: &roundTripFunc{
					f: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusNotFound,
						}, nil
					},
				},
			},
			givenURL: "https://example.com/wasm.wasm",
			assert: func(t *testing.T, r io.Reader, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to download WASM file")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := new(bytes.Buffer)
			err := NewHTTPSDownloader(tt.givenHTTPClient, tt.givenURL).DownloadWASM(ctx, b)
			tt.assert(t, b, err)
		})
	}
}
