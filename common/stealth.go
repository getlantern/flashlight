package common

import (
	"sync/atomic"
)

var (
	inStealthMode    int64
	forceStealthMode int64
)

// SetStealthMode enables/disables stealth mode
func SetStealthMode(stealthMode bool) {
	stealthModeInt := int64(0)
	if stealthMode {
		stealthModeInt = 1
	}
	atomic.StoreInt64(&inStealthMode, stealthModeInt)
}

// ForceStealthMode forces stealth mode whether or not it's enabled via configuration
func ForceStealthMode() {
	atomic.StoreInt64(&forceStealthMode, 1)
}

// InStealthMode checks if stealth mode is enabled
func InStealthMode() bool {
	return atomic.LoadInt64(&inStealthMode) == 1 || atomic.LoadInt64(&forceStealthMode) == 1
}
