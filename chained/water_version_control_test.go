package chained

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestNewWaterVersionControl(t *testing.T) {
	var tests = []struct {
		name   string
		assert func(t *testing.T, dir string, r io.ReadCloser, err error)
		setup  func(t *testing.T, ctrl *gomock.Controller, dir string) waterWASMDownloader
	}{
		{
			name: "it should call downloadWASM when the file does not exist",
			assert: func(t *testing.T, dir string, r io.ReadCloser, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, r)
				defer r.Close()

				b, err := io.ReadAll(r)
				require.NoError(t, err)
				assert.Equal(t, "test", string(b))

				_, err = os.Stat(filepath.Join(dir, "test.wasm"))
				assert.NoError(t, err)

				_, err = os.Stat(filepath.Join(dir, "test.last-loaded"))
				assert.NoError(t, err)
			},
			setup: func(t *testing.T, ctrl *gomock.Controller, _ string) waterWASMDownloader {
				downloader := NewMockwaterWASMDownloader(ctrl)
				downloader.EXPECT().DownloadWASM(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, w io.Writer) error {
					assert.NotNil(t, ctx)
					assert.NotNil(t, w)
					_, err := w.Write([]byte("test"))
					require.NoError(t, err)
					return nil
				})
				return downloader
			},
		},
		{
			name: "it should delete outdated WASM files after marking it as used",
			assert: func(t *testing.T, dir string, r io.ReadCloser, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, r)
				defer r.Close()

				b, err := io.ReadAll(r)
				require.NoError(t, err)
				assert.Equal(t, "test", string(b))

				_, err = os.Stat(filepath.Join(dir, "test.wasm"))
				assert.NoError(t, err)

				_, err = os.Stat(filepath.Join(dir, "test.last-loaded"))
				assert.NoError(t, err)

				// assert old-test does not exist
				_, err = os.Stat(filepath.Join(dir, "old-test.wasm"))
				assert.Error(t, os.ErrNotExist, err)

				_, err = os.Stat(filepath.Join(dir, "old-test.last-loaded"))
				assert.Error(t, os.ErrNotExist, err)

			},
			setup: func(t *testing.T, ctrl *gomock.Controller, dir string) waterWASMDownloader {
				// create test.wasm at dir
				f, err := os.Create(filepath.Join(dir, "old-test.wasm"))
				require.NoError(t, err)
				_, err = f.WriteString("test")
				require.NoError(t, err)
				require.NoError(t, f.Close())

				// create test.last-loaded at dir with time older than 7 days
				f, err = os.Create(filepath.Join(dir, "old-test.last-loaded"))
				require.NoError(t, err)
				unixTime := time.Now().UTC().AddDate(0, 0, -8).Unix()
				oldTime := strconv.FormatInt(unixTime, 10)
				_, err = f.WriteString(oldTime)
				require.NoError(t, err)
				require.NoError(t, f.Close())

				downloader := NewMockwaterWASMDownloader(ctrl)
				downloader.EXPECT().DownloadWASM(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, w io.Writer) error {
					assert.NotNil(t, ctx)
					assert.NotNil(t, w)
					_, err := w.Write([]byte("test"))
					require.NoError(t, err)
					return nil
				})
				return downloader
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "water")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			downloader := tt.setup(t, gomock.NewController(t), dir)
			vc := newWaterVersionControl(dir)
			require.NotNil(t, vc)
			require.NotEmpty(t, vc.dir)

			ctx := context.Background()
			r, err := vc.GetWASM(ctx, "test", downloader)

			tt.assert(t, dir, r, err)
		})
	}
}
