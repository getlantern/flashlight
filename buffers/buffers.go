package buffers

import (
	"github.com/getlantern/lampshade"
)

// This buffer applies to the Tunnel network extension only:
// https://github.com/getlantern/lantern-ios/blob/58488afc5696aaaa0b956b97bbb9a16bbe856b1e/Tunnel/Info.plist#L1
//
// Memory limit for network extensions in iOS15 is 50MB
// (https://developer.apple.com/forums/thread/73148?page=2).
// We use 1MB and that's enough here. Most of our users (as of 2022-10-25)
// were iOS15 so we're safe to use this number.
//
// This buffer pool is really just for optimization. Even with a minimal pool,
// things will work but with a few more GC hits.
const (
	maxBufferBytes = 1 * 1024 * 1024
)

// Pool is a pool of buffers
var Pool = lampshade.NewBufferPool(maxBufferBytes)

// MaxBufferBytes exposes the configured maxBufferBytes
func MaxBufferBytes() int {
	return maxBufferBytes
}
