// +build !headless

package main

import (
	"fmt"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/app"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/ui"
)

var menu struct {
	enable  bool
	on      bool
	toggle  *systray.MenuItem
	show    *systray.MenuItem
	upgrade *systray.MenuItem
}

func runOnSystrayReady(a *app.App, f func()) {
	// Typically, systray.Quit will actually be what causes the app to exit, but
	// in the case of an uncaught Fatal error, the app will exit before the
	// systray and we need it to call systray.Quit().
	a.AddExitFuncToEnd(func() {
		systray.Quit()
	})

	systray.Run(f, func() {
		a.Exit(nil)
		err := a.WaitForExit()
		if err != nil {
			log.Errorf("Error exiting app: %v", err)
		}
	})
}

func configureSystemTray(a *app.App) error {
	menu.enable = a.ShowUI
	if !menu.enable {
		return nil
	}
	onIcon, err := icons.Asset("icons/16on.ico")
	if err != nil {
		return fmt.Errorf("Unable to load on icon for system tray: %v", err)
	}
	offIcon, err := icons.Asset("icons/16off.ico")
	if err != nil {
		return fmt.Errorf("Unable to load off icon for system tray: %v", err)
	}
	systray.SetTooltip("Lantern")
	menu.on = a.IsOn()
	if menu.on {
		menu.toggle = systray.AddMenuItem(i18n.T("TRAY_TURN_OFF"), i18n.T("TRAY_TURN_OFF"))
		systray.SetIcon(onIcon)
	} else {
		menu.toggle = systray.AddMenuItem(i18n.T("TRAY_TURN_ON"), i18n.T("TRAY_TURN_ON"))
		systray.SetIcon(offIcon)
	}
	menu.show = systray.AddMenuItem(i18n.T("TRAY_SHOW_LANTERN"), i18n.T("TRAY_SHOW_LANTERN"))
	menu.upgrade = systray.AddMenuItem(i18n.T("TRAY_UPGRADE_TO_PRO"), i18n.T("TRAY_UPGRADE_TO_PRO"))
	pro.OnProStatusChange(func(isPro bool) {
		if isPro {
			menu.upgrade.Hide()
		} else {
			menu.upgrade.Show()
		}
	})
	go func() {
		for {
			select {
			case <-menu.toggle.ClickedCh:
				if menu.on {
					a.TurnOff()
					systray.SetIcon(offIcon)
					menu.on = false
				} else {
					a.TurnOn()
					systray.SetIcon(onIcon)
					menu.on = true
				}
				setOnOffLabels()
			case <-menu.show.ClickedCh:
				ui.ShowRoot("show-lantern", "tray")
			case <-menu.upgrade.ClickedCh:
				ui.Show(ui.AddToken("/")+"#/plans", "proupgrade", "tray")
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
	setOnOffLabels()
}

func setOnOffLabels() {
	if menu.on {
		menu.toggle.SetTitle(i18n.T("TRAY_TURN_OFF"))
		menu.toggle.SetTooltip(i18n.T("TRAY_TURN_OFF"))
	} else {
		menu.toggle.SetTitle(i18n.T("TRAY_TURN_ON"))
		menu.toggle.SetTooltip(i18n.T("TRAY_TURN_ON"))
	}
}
