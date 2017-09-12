// +build !headless

package main

import (
	"fmt"
	"sync"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/app"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/ui"
)

var menu struct {
	enable     bool
	on         bool
	hitDataCap bool
	isPro      bool
	statusMx   sync.RWMutex
	status     *systray.MenuItem
	toggle     *systray.MenuItem
	show       *systray.MenuItem
	upgrade    *systray.MenuItem
	quit       *systray.MenuItem
}

var (
	onIcon  []byte
	offIcon []byte
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
	onIcon, err = icons.Asset("icons/16on.ico")
	if err != nil {
		return fmt.Errorf("Unable to load on icon for system tray: %v", err)
	}
	offIcon, err = icons.Asset("icons/16off.ico")
	if err != nil {
		return fmt.Errorf("Unable to load off icon for system tray: %v", err)
	}
	systray.SetTooltip("Lantern")
	menu.status = systray.AddMenuItem("", "")
	menu.status.Disable()
	menu.on = a.IsOn()
	menu.toggle = systray.AddMenuItem("", "")
	menu.show = systray.AddMenuItem(i18n.T("TRAY_SHOW_LANTERN"), i18n.T("TRAY_SHOW_LANTERN"))
	menu.upgrade = systray.AddMenuItem(i18n.T("TRAY_UPGRADE_TO_PRO"), i18n.T("TRAY_UPGRADE_TO_PRO"))
	systray.AddSeparator()
	menu.quit = systray.AddMenuItem(i18n.T("TRAY_QUIT"), i18n.T("TRAY_QUIT"))
	app.AddDataCapListener(func() {
		menu.statusMx.Lock()
		menu.hitDataCap = true
		menu.statusMx.Unlock()
		updateStatus()
	})
	pro.OnProStatusChange(func(isPro bool) {
		menu.statusMx.Lock()
		menu.isPro = isPro
		menu.statusMx.Unlock()
		updateStatus()
	})
	updateStatus()
	go func() {
		for {
			select {
			case <-menu.toggle.ClickedCh:
				menu.statusMx.Lock()
				menu.on = !menu.on
				on := menu.on
				menu.statusMx.Unlock()
				if on {
					a.TurnOn()
				} else {
					a.TurnOff()
				}
				updateStatus()
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
	updateStatus()
}

func updateStatus() {
	menu.statusMx.RLock()
	on := menu.on
	hitDataCap := menu.hitDataCap
	isPro := menu.isPro
	menu.statusMx.RUnlock()

	var status string
	if hitDataCap && !isPro {
		status = i18n.T("TRAY_STATUS", i18n.T("TRAY_STATUS_BANDWIDTH_LIMITED"))
	} else if on {
		status = i18n.T("TRAY_STATUS", i18n.T("TRAY_STATUS_CONNECTED"))
	} else {
		status = i18n.T("TRAY_STATUS", i18n.T("TRAY_STATUS_DISCONNECTED"))
	}
	menu.status.SetTitle(status)
	menu.status.SetTooltip(status)

	if isPro {
		menu.upgrade.Hide()
	} else {
		menu.upgrade.Show()
	}

	if on {
		systray.SetIcon(onIcon)
		menu.toggle.SetTitle(i18n.T("TRAY_TURN_OFF"))
		menu.toggle.SetTooltip(i18n.T("TRAY_TURN_OFF"))
	} else {
		systray.SetIcon(offIcon)
		menu.toggle.SetTitle(i18n.T("TRAY_TURN_ON"))
		menu.toggle.SetTooltip(i18n.T("TRAY_TURN_ON"))
	}
}
