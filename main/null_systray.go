// +build headless

package main

import (
	"os"

	"github.com/getlantern/flashlight/desktop"
)

func runOnSystrayReady(standalone bool, a *desktop.App, f func()) {
	f()
	err := a.WaitForExit()
	if err != nil {
		log.Error(err)
	}
	os.Exit(0)
}

func quitSystray() {
}

func configureSystemTray(a *desktop.App) error {
	return nil
}

func refreshSystray(language string) {
}
