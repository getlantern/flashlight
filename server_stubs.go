// +build android

package main

import (
	"runtime"

	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/log"
)

// runServerProxy is not implemented.
func runServerProxy(cfg *config.Config) {
	log.Debugf("runServerProxy not implemented in %s/%s.", runtime.GOOS, runtime.GOARCH)
}
