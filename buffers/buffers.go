package buffers

import (
	"github.com/getlantern/lampshade"
)

// Pool is a pool of buffers
var Pool = lampshade.NewBufferPool(maxBufferBytes)

// MaxBufferBytes exposes the configured maxBufferBytes
func MaxBufferBytes() int {
	return maxBufferBytes
}
