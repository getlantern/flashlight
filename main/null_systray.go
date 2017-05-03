// +build headless

package main

import "github.com/getlantern/flashlight/app"

func runOnSystrayReady(a *app.App, f func()) {
	f()
}

func quitSystray() {
}

func configureSystemTray(a *app.App) error {
	return nil
}

func refreshSystray(language string) {
}
