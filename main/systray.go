// +build !headless

package main

import (
	"fmt"
	"sync"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/app"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/ui"
)

var menu struct {
	enable  bool
	st      app.Status
	stMx    sync.RWMutex
	status  *systray.MenuItem
	toggle  *systray.MenuItem
	show    *systray.MenuItem
	upgrade *systray.MenuItem
	quit    *systray.MenuItem
}

var (
	iconConnected    []byte
	iconDisconnected []byte
	iconIssue        []byte
)

func runOnSystrayReady(a *app.App, f func()) {
	// Typically, systray.Quit will actually be what causes the app to exit, but
	// in the case of an uncaught Fatal error, the app will exit before the
	// systray and we need it to call systray.Quit().
	a.AddExitFuncToEnd(systray.Quit)

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

	var err error
	iconConnected, err = icons.Asset("connected_16.ico")
	if err != nil {
		return fmt.Errorf("Unable to load connected icon for system tray: %v", err)
	}
	iconDisconnected, err = icons.Asset("disconnected_16.ico")
	if err != nil {
		return fmt.Errorf("Unable to load disconnected icon for system tray: %v", err)
	}
	iconIssue, err = icons.Asset("issue_16.ico")
	if err != nil {
		return fmt.Errorf("Unable to load issue icon for system tray: %v", err)
	}

	systray.SetTooltip("Lantern")
	menu.status = systray.AddMenuItem("", "")
	menu.status.Disable()
	menu.toggle = systray.AddMenuItem("", "")
	menu.show = systray.AddMenuItem(i18n.T("TRAY_SHOW_LANTERN"), i18n.T("TRAY_SHOW_LANTERN"))
	menu.upgrade = systray.AddMenuItem(i18n.T("TRAY_UPGRADE_TO_PRO"), i18n.T("TRAY_UPGRADE_TO_PRO"))
	systray.AddSeparator()
	menu.quit = systray.AddMenuItem(i18n.T("TRAY_QUIT"), i18n.T("TRAY_QUIT"))
	go func() {
		for status := range a.StatusUpdates() {
			menu.stMx.Lock()
			menu.st = status
			menu.stMx.Unlock()
			statusUpdated()
		}
	}()

	go func() {
		for {
			select {
			case <-menu.toggle.ClickedCh:
				menu.stMx.Lock()
				on := menu.st.On
				menu.stMx.Unlock()
				if on {
					a.TurnOff()
				} else {
					a.TurnOn()
				}
			case <-menu.show.ClickedCh:
				ui.ShowRoot("show-lantern", "tray")
			case <-menu.upgrade.ClickedCh:
				ui.Show(ui.AddToken("/")+"#/plans", "proupgrade", "tray")
			case <-menu.quit.ClickedCh:
				systray.Quit()
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
	statusUpdated()
}

func statusUpdated() {
	menu.stMx.RLock()
	st := menu.st
	menu.stMx.RUnlock()

	switch st.String() {
	case app.STATUS_CONNECTING:
		systray.SetIcon(iconDisconnected)
	case app.STATUS_CONNECTED:
		systray.SetIcon(iconConnected)
	case app.STATUS_DISCONNECTED:
		systray.SetIcon(iconDisconnected)
	case app.STATUS_THROTTLED:
		systray.SetIcon(iconIssue)
	}
	status := i18n.T("TRAY_STATUS", i18n.T("status."+st.String()))
	menu.status.SetTitle(status)
	menu.status.SetTooltip(status)

	if st.IsPro {
		menu.upgrade.Hide()
	} else {
		menu.upgrade.Show()
	}

	if st.On {
		menu.toggle.SetTitle(i18n.T("TRAY_TURN_OFF"))
		menu.toggle.SetTooltip(i18n.T("TRAY_TURN_OFF"))
	} else {
		menu.toggle.SetTitle(i18n.T("TRAY_TURN_ON"))
		menu.toggle.SetTooltip(i18n.T("TRAY_TURN_ON"))
	}
}
