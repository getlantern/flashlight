package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
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

func TestParentContext(t *testing.T) {
	t.Setenv("TRACEPARENT", "00-323cc51c1b2848fdab828f68b76ff811-0120f713aaa5e6f2-01")
	ctx := parentContext()
	require.NotNil(t, ctx)
	sc := trace.SpanContextFromContext(ctx)
	assert.Equal(t, "323cc51c1b2848fdab828f68b76ff811", sc.TraceID().String())
	assert.Equal(t, "0120f713aaa5e6f2", sc.SpanID().String())
	assert.True(t, sc.IsSampled())
	for _, invalid := range []string{"", "garbage", "00-00000000000000000000000000000000-0120f713aaa5e6f2-01"} {
		t.Setenv("TRACEPARENT", invalid)
		assert.Nil(t, parentContext(), invalid)
	}
}
