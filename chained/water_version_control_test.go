package chained

import (
	"os"
	"path"
	"testing"

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
		assert         func(t *testing.T, vc VersionControl, err error)
		setup          func()
	}{
		{
			name:           "it should return a new versionControl successfully when receiving a configDir and it should create the directory if it doesn't exist",
			givenConfigDir: inexistentPath,
			assert: func(t *testing.T, vc VersionControl, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, vc)
				assert.DirExists(t, inexistentPath)
			},
		},
		{
			name:           "it should return a error if the directory contains a file with invalid wasm name",
			givenConfigDir: configDir,
			assert: func(t *testing.T, vc VersionControl, err error) {
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
			assert: func(t *testing.T, vc VersionControl, err error) {
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
			vc, err := NewVersionControl(tt.givenConfigDir)
			tt.assert(t, vc, err)
		})
	}
}
