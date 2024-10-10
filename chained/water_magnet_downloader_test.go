package chained

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMagnetDownloadWASM run a integration test for validating if the magnet downloader is working as expected.
func TestMagnetDownloadWASM(t *testing.T) {
	ctx := context.Background()
	var tests = []struct {
		name           string
		givenMagnetURL string
		assert         func(t *testing.T, r io.Reader, err error)
	}{
		{
			name:           "downloading learning python in a week",
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
			b := new(bytes.Buffer)
			err := NewMagnetDownloader(tt.givenMagnetURL).DownloadWASM(ctx, b)
			tt.assert(t, b, err)
		})
	}
}
