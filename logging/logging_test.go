package logging

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
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

type nopCloser struct {
	io.Writer
	closeCalled bool
}

func (c *nopCloser) Close() error { c.closeCalled = true; return nil }

func TestNonStopWriter(t *testing.T) {
	b, g := &badWriter{}, &goodWriter{}
	ncb, ncg := &nopCloser{Writer: b}, &nopCloser{Writer: g}
	ns := NonStopWriteCloser(ncb, ncg)
	ns.Write([]byte("1234"))
	assert.Equal(t, 4, g.counter, "Should write to all writers even when error encountered")
	ns.Close()
	assert.False(t, ncb.closeCalled)
	assert.False(t, ncg.closeCalled)
}

func TestPipedWriteCloserWriteProperly(t *testing.T) {
	entry := []byte("abcd\n")
	var b bytes.Buffer
	w := newPipedWriteCloser(&nopCloser{Writer: &b}, 100)
	for i := 0; i < 100; i++ {
		w.Write(entry)
	}
	w.Close()
	assert.Equal(t, b.Bytes(), bytes.Repeat(entry, 100))
}

func TestPipedWriteCloserSkipMessages(t *testing.T) {
	entry := []byte("abcd\n")
	var b bytes.Buffer
	w := newPipedWriteCloser(&nopCloser{Writer: &b}, 10)
	for i := 0; i < 999; i++ {
		w.Write(entry)
	}
	time.Sleep(time.Millisecond)
	// Write one last entry to trigger the printing of the skipped message
	// count, if any
	w.Write(entry)
	w.Close()
	assert.Contains(t, string(b.Bytes()), "message(s) skipped...")
	assert.Equal(t, 1000, countMessages(b.Bytes()))
}

func countMessages(b []byte) int {
	lines := bytes.Split(b, []byte("\n"))
	var n int
	re := regexp.MustCompile("\\d+")
	for _, l := range lines {
		found := re.Find(l)
		if found == nil {
			if len(l) > 0 { // exclude the latest '\n'
				n++
			}
		} else {
			i, _ := strconv.Atoi(string(found))
			n += i
		}
	}
	return n
}

func TestCloseAndInit(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "logging_test")
	if !assert.NoError(t, err) {
		return
	}
	defer os.RemoveAll(tmpDir)
	for i := 0; i < 10; i++ {
		EnableFileLogging(tmpDir)
		Close()
	}
}
