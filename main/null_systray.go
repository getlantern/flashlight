// +build headless

package main

import (
	"os"

	"github.com/getlantern/flashlight/app"
)

func runOnSystrayReady(a *app.App, f func(quit func())) {
	f(func() {
		a.Exit(nil)
	})
	err := a.WaitForExit()
	if err != nil {
		log.Error(err)
	}
	os.Exit(0)
}

func quitSystray() {
}

func configureSystemTray(a *app.App) error {
	return nil
}

func refreshSystray(language string) {
}
