package logging

import (
	"fmt"
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

type BadWriter struct{}
type GoodWriter struct{ counter int }

func (w *BadWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("Fail intentionally")
}

func (w *GoodWriter) Write(p []byte) (int, error) {
	w.counter = len(p)
	return w.counter, nil
}
