// +build !headless

package main

import (
	"fmt"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/app"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/ui"
)

var menu struct {
	enable bool
	show   *systray.MenuItem
	quit   *systray.MenuItem
}

func runOnSystrayReady(a *app.App, f func()) {
	systray.Run(f, func() {
		log.Debug("Exiting")
		a.Exit(nil)
		log.Debug("Done exiting")
	})
}

func configureSystemTray(a *app.App) error {
	menu.enable = a.ShowUI
	if !menu.enable {
		return nil
	}
	icon, err := icons.Asset("icons/16on.ico")
	if err != nil {
		return fmt.Errorf("Unable to load icon for system tray: %v", err)
	}
	systray.SetIcon(icon)
	systray.SetTooltip("Lantern")
	menu.show = systray.AddMenuItem(i18n.T("TRAY_SHOW_LANTERN"), i18n.T("SHOW"))
	menu.quit = systray.AddMenuItem(i18n.T("TRAY_QUIT"), i18n.T("QUIT"))
	go func() {
		for {
			select {
			case <-menu.show.ClickedCh:
				ui.Show("show-lantern", "tray")
			case <-menu.quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	return nil
}

func refreshSystray(language string) {
	if !menu.enable {
		return
	}
	if err := i18n.SetLocale(language); err != nil {
		log.Errorf("i18n.SetLocale(%s) failed: %q", language, err)
		return
	}
	menu.show.SetTitle(i18n.T("TRAY_SHOW_LANTERN"))
	menu.show.SetTooltip(i18n.T("SHOW"))
	menu.quit.SetTitle(i18n.T("TRAY_QUIT"))
	menu.quit.SetTooltip(i18n.T("QUIT"))
}
