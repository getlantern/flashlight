package util

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Expand expands the path to include the home directory if the path
// is prefixed with `~`. If it isn't prefixed with `~`, the path is
// returned as-is.
func ExpandPath(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] != '~' {
		return path
	}
	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return path
	}
	return filepath.Join(os.Getenv("HOME"), path[1:])
}

func WaitForWaitGroup(wg *sync.WaitGroup, timeout time.Duration) (ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	select {
	case <-ctx.Done():
		return false
	case <-ch:
		return true
	}
}

type IoNopCloser struct{}

func (IoNopCloser) Close() error {
	return nil
}

func MustReadFile(filePath string) []byte {
	b, err := os.ReadFile(filePath)
	if err != nil {
		panic(fmt.Errorf("Failed to read file %s: %w", filePath, err))
	}
	return b
}
