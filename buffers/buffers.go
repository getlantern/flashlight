package buffers

import (
	"github.com/getlantern/lampshade"
)

// Pool is a pool of buffers
var Pool = lampshade.NewBufferPool(1000)
