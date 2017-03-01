package buffers

import (
	"github.com/getlantern/lampshade"
)

const (
	maxBufferBytes = 30 * 1024 * 1024
)

// Pool is a pool of buffers
var Pool = lampshade.NewBufferPool(maxBufferBytes)
