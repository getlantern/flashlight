package chained

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
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
		setup          func(ctrl *gomock.Controller, downloader *magnetDownloader) WASMDownloader
		givenCtx       context.Context
		givenMagnetURL string
		assert         func(t *testing.T, r io.Reader, err error)
	}{
		{
			name: "should return success when download is successful",
			setup: func(ctrl *gomock.Controller, downloader *magnetDownloader) WASMDownloader {
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
				// b, err := io.ReadAll(r)
				// require.NoError(t, err)
				// assert.Equal(t, "hello world", string(b))
			},
		},
		{
			name:           "integration test",
			givenCtx:       ctx,
			givenMagnetURL: "magnet:?xt=urn:btih:34F2A1FA5CD593C394C6E5B5B83B92A7165EA9A9&dn=Learn+Python+In+A+Week+And+Master+It&tr=http%3A%2F%2Fp4p.arenabg.com%3A1337%2Fannounce&tr=udp%3A%2F%2F47.ip-51-68-199.eu%3A6969%2Fannounce&tr=udp%3A%2F%2F9.rarbg.me%3A2780%2Fannounce&tr=udp%3A%2F%2F9.rarbg.to%3A2710%2Fannounce&tr=udp%3A%2F%2F9.rarbg.to%3A2730%2Fannounce&tr=udp%3A%2F%2F9.rarbg.to%3A2920%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Fopentracker.i2p.rocks%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.cyberia.is%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.dler.org%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.internetwarriors.net%3A1337%2Fannounce&tr=udp%3A%2F%2Ftracker.leechers-paradise.org%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.openbittorrent.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337&tr=udp%3A%2F%2Ftracker.pirateparty.gr%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.tiny-vps.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.torrent.eu.org%3A451%2Fannounce",
			assert: func(t *testing.T, r io.Reader, err error) {
				assert.NoError(t, err)
				b, err := io.ReadAll(r)
				require.NoError(t, err)
				hashsum := sha256.Sum256(b)
				expectedHashsum := "0e87e6161860dfcdd54842d2cb468df8921ecb47650b66d3f6c6f25c1bc173a9"
				assert.Equal(t, expectedHashsum, fmt.Sprintf("%x", hashsum))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloader, err := NewMagnetDownloader(tt.givenCtx, tt.givenMagnetURL)
			require.NoError(t, err)
			defer downloader.Close()
			if tt.setup != nil {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				tt.setup(ctrl, downloader.(*magnetDownloader))
			}

			b := new(bytes.Buffer)
			err = downloader.DownloadWASM(ctx, b)
			tt.assert(t, b, err)
		})
	}
}
