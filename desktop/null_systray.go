// +build headless

package desktop

import (
	"os"
)

func RunOnSystrayReady(standalone bool, a *App, f func()) {
	f()
	err := a.WaitForExit()
	if err != nil {
		log.Errorf("Lantern stopped with error %v", err)
		os.Exit(-1)
	}
	log.Debug("Lantern stopped")
	os.Exit(0)
}

func QuitSystray(a *App) {
	a.Exit(nil)
}

func configureSystemTray(a *App) error {
	return nil
}

func refreshSystray(language string) {
}
