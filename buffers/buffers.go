package buffers

import (
	"runtime"
	"strings"

	"github.com/getlantern/lampshade"
)

var Pool lampshade.BufferPool

func init() {
	maxBufferBytes := 1024 * 1024
	onDesktop := !strings.HasPrefix(runtime.GOARCH, "arm")
	if onDesktop {
		// use larger buffer pool on desktop and android
		maxBufferBytes = 30 * maxBufferBytes
	}
	Pool = lampshade.NewBufferPool(maxBufferBytes)
}
