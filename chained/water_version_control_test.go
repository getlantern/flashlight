package chained

import (
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVersionControl(t *testing.T) {
	configDir, err := os.MkdirTemp("", "water")
	require.NoError(t, err)
	defer os.RemoveAll(configDir)

	inexistentPath := path.Join(configDir, "aaaa")
	var tests = []struct {
		name           string
		givenConfigDir string
		assert         func(t *testing.T, vc *waterVersionControl, err error)
		setup          func()
	}{
		{
			name:           "it should return a new versionControl successfully when receiving a configDir and it should create the directory if it doesn't exist",
			givenConfigDir: inexistentPath,
			assert: func(t *testing.T, vc *waterVersionControl, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, vc)
				assert.DirExists(t, inexistentPath)
			},
		},
		{
			name:           "it should return a error if the directory contains a file with invalid wasm name",
			givenConfigDir: configDir,
			assert: func(t *testing.T, vc *waterVersionControl, err error) {
				assert.Error(t, err)
				assert.Nil(t, vc)
				os.Remove(path.Join(configDir, "aaaaa.wasm"))
			},
			setup: func() {
				_, err := os.Create(path.Join(configDir, "aaaaa.wasm"))
				require.NoError(t, err)
			},
		},
		{
			name:           "it should return a versionControl with the wasm files available",
			givenConfigDir: configDir,
			assert: func(t *testing.T, vc *waterVersionControl, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, vc)
			},
			setup: func() {
				_, err := os.Create(path.Join(configDir, "plain.v1.tinygo.wasm"))
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			vc, err := newWaterVersionControl(tt.givenConfigDir)
			tt.assert(t, vc, err)
		})
	}
}

func TestCommit(t *testing.T) {

	var tests = []struct {
		name           string
		givenTransport string
		setup          func(t *testing.T, configDir string)
		assert         func(t *testing.T, configDir string, err error)
	}{
		{
			name:           "it should add one register to the history",
			givenTransport: "plain.v1.tinygo.wasm",
			assert: func(t *testing.T, configDir string, err error) {
				assert.NoError(t, err)
				historyCSV := filepath.Join(configDir, "history.csv")
				assert.FileExists(t, historyCSV)

				f, err := os.Open(historyCSV)
				require.NoError(t, err)
				defer f.Close()

				history := []history{}
				require.NoError(t, gocsv.UnmarshalFile(f, &history))
				assert.Len(t, history, 1)
				assert.Equal(t, "plain.v1.tinygo.wasm", history[0].Transport)
				assert.NotEmpty(t, history[0].LastTimeLoaded)
			},
		},
		{
			name:           "it should update the last time loaded of the transport",
			givenTransport: "plain.v1.tinygo.wasm",
			setup: func(t *testing.T, configDir string) {
				historyCSV := filepath.Join(configDir, "history.csv")
				history := []history{
					{
						Transport:      "plain.v1.tinygo.wasm",
						LastTimeLoaded: time.Now().Add(-1 * time.Hour),
					},
				}

				f, err := os.Create(historyCSV)
				require.NoError(t, err)
				defer f.Close()

				require.NoError(t, gocsv.MarshalFile(&history, f))
			},
			assert: func(t *testing.T, configDir string, err error) {
				assert.NoError(t, err)
				historyCSV := filepath.Join(configDir, "history.csv")
				assert.FileExists(t, historyCSV)

				f, err := os.Open(historyCSV)
				require.NoError(t, err)
				defer f.Close()

				history := []history{}
				require.NoError(t, gocsv.UnmarshalFile(f, &history))
				assert.Len(t, history, 1)
				assert.Equal(t, "plain.v1.tinygo.wasm", history[0].Transport)
				assert.NotEmpty(t, history[0].LastTimeLoaded)
				assert.True(t, history[0].LastTimeLoaded.After(time.Now().Add(-1*time.Minute)))
			},
		},
		{
			name:           "it should delete the outdated wasm files",
			givenTransport: "plain.v2.tinygo.wasm",
			setup: func(t *testing.T, configDir string) {
				historyCSV := filepath.Join(configDir, "history.csv")
				history := []history{
					{
						Transport:      "plain.v1.tinygo.wasm",
						LastTimeLoaded: time.Now().Add(-8 * 24 * time.Hour),
					},
				}

				f, err := os.Create(historyCSV)
				require.NoError(t, err)
				defer f.Close()

				require.NoError(t, gocsv.MarshalFile(&history, f))

				_, err = os.Create(path.Join(configDir, "plain.v1.tinygo.wasm"))
				require.NoError(t, err)
			},
			assert: func(t *testing.T, configDir string, err error) {
				assert.NoError(t, err)
				historyCSV := filepath.Join(configDir, "history.csv")
				assert.FileExists(t, historyCSV)

				f, err := os.Open(historyCSV)
				require.NoError(t, err)
				defer f.Close()

				history := []history{}
				require.NoError(t, gocsv.UnmarshalFile(f, &history))
				assert.Len(t, history, 1)
				assert.Equal(t, "plain.v2.tinygo.wasm", history[0].Transport)
				assert.NotEmpty(t, history[0].LastTimeLoaded)

				assert.NoFileExists(t, path.Join(configDir, "plain.v1.tinygo.wasm"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir, err := os.MkdirTemp("", "water")
			require.NoError(t, err)
			defer os.RemoveAll(configDir)

			if tt.setup != nil {
				tt.setup(t, configDir)
			}

			vc, err := newWaterVersionControl(configDir)
			require.NoError(t, err)

			err = vc.Commit(tt.givenTransport)
			tt.assert(t, configDir, err)
		})
	}
}
