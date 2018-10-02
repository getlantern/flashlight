package logging

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test to make sure user agent registration, listening, etc is all working.
func TestUserAgent(t *testing.T) {
	agent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/50.0.2661.86 Safari/537.36"

	// Do an initial register just to test the duplicate agent paths.
	RegisterUserAgent(agent)

	go func() {
		RegisterUserAgent(agent)
	}()

	time.Sleep(200 * time.Millisecond)

	agents := getSessionUserAgents()

	assert.True(t, strings.Contains(agents, "AppleWebKit"), "Expected agent not in "+agents)
}

type badWriter struct{}
type goodWriter struct{ counter int }

func (w *badWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("Fail intentionally")
}

func (w *goodWriter) Write(p []byte) (int, error) {
	w.counter = len(p)
	return w.counter, nil
}

func TestNonStopWriter(t *testing.T) {
	b, g := badWriter{}, goodWriter{}
	ns := newNonStopWriter(&b, &g)
	ns.Write([]byte("1234"))
	assert.Equal(t, 4, g.counter, "Should write to all writers even when error encountered")
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

func TestPipedWriteCloserWriteProperly(t *testing.T) {
	entry := []byte("abcd")
	var b bytes.Buffer
	w := newPipedWriteCloser(nopCloser{&b}, 100)
	for i := 0; i < 100; i++ {
		w.Write(entry)
	}
	w.Close()
	assert.Equal(t, b.Bytes(), bytes.Repeat(entry, 100))
}
