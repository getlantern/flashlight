package chained

import (
	"bytes"
	"context"
	"io"
	"testing"

	events "github.com/anacrolix/chansync/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

// TestMagnetDownloadWASM run a integration test for validating if the magnet downloader is working as expected.
func TestMagnetDownloadWASM(t *testing.T) {
	ctx := context.Background()
	var tests = []struct {
		name           string
		setup          func(ctrl *gomock.Controller, downloader *waterMagnetDownloader) waterWASMDownloader
		givenCtx       context.Context
		givenMagnetURL string
		assert         func(t *testing.T, r io.Reader, err error)
	}{
		{
			name: "should return success when download is successful",
			setup: func(ctrl *gomock.Controller, downloader *waterMagnetDownloader) waterWASMDownloader {
				downloader.Close()
				torrentClient := NewMockTorrentClient(ctrl)
				torrentInfo := NewMockTorrentInfo(ctrl)

				torrentReader := NewMockReader(ctrl)
				// torrent reader receive Read calls from io.Copy, it should be able to store the message hello world
				// and finish the Read call (along with the io.Copy)
				torrentReader.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
					copy(p, []byte("hello world"))
					return len(p), io.EOF
				}).AnyTimes()

				torrentClient.EXPECT().AddMagnet(downloader.magnetURL).Return(torrentInfo, nil).Times(1)
				done := make(chan struct{})
				// send done
				torrentInfo.EXPECT().GotInfo().DoAndReturn(func() events.Done {
					defer func() {
						close(done)
					}()
					return done
				}).Times(1)
				torrentInfo.EXPECT().NewReader().Return(torrentReader).AnyTimes()
				torrentClient.EXPECT().Close().Return(nil).AnyTimes()
				downloader.client = torrentClient
				return downloader
			},
			givenCtx:       ctx,
			givenMagnetURL: "",
			assert: func(t *testing.T, r io.Reader, err error) {
				assert.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloader, err := newWaterMagnetDownloader(tt.givenCtx, tt.givenMagnetURL)
			require.NoError(t, err)
			defer downloader.Close()
			if tt.setup != nil {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				tt.setup(ctrl, downloader.(*waterMagnetDownloader))
			}

			b := new(bytes.Buffer)
			err = downloader.DownloadWASM(ctx, b)
			tt.assert(t, b, err)
		})
	}
}
