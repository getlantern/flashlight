// +build headless

package main

import "github.com/getlantern/flashlight/desktop"

func runOnSystrayReady(a *desktop.App, f func()) {
	f()
}

func quitSystray() {
}

func configureSystemTray(a *desktop.App) error {
	return nil
}

func refreshSystray(language string) {
}
