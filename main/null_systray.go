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
		log.Errorf("Lantern stopped with error %v", err)
		os.Exit(-1)
	}
	log.Debug("Lantern stopped")
	os.Exit(0)
}

func quitSystray(a *desktop.App) {
	a.Exit(nil)
}

func configureSystemTray(a *desktop.App) error {
	return nil
}

func refreshSystray(language string) {
}
