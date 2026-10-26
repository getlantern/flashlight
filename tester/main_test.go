package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidTimeoutClearsPreviousResult(t *testing.T) {
	dir := t.TempDir()
	names := []string{"success", "fetch.json", "connection.txt", "output.txt", "timing.txt"}
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("previous run"), 0o600))
	}
	t.Setenv("FETCH_TIMEOUT_SECONDS", "invalid")
	err := performLanternPing("https://example.com", "run", "device", 1, "token", dir, true, "key")
	require.Error(t, err)
	for _, name := range names {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
}
