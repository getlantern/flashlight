// +build !headless

package main

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/app"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ui"
)

var menu struct {
	enable  bool
	st      stats.Stats
	stMx    sync.RWMutex
	status  *systray.MenuItem
	toggle  *systray.MenuItem
	show    *systray.MenuItem
	upgrade *systray.MenuItem
	quit    *systray.MenuItem
}

var (
	iconsByName = make(map[string][]byte)
)

func runOnSystrayReady(a *app.App, f func()) {
	// Typically, systray.Quit will actually be what causes the app to exit, but
	// in the case of an uncaught Fatal error, the app will exit before the
	// systray and we need it to call systray.Quit().
	a.AddExitFuncToEnd(func() {
		systray.Quit()
	})

	systray.Run(f, func() {
		if a.Exit(nil) {
			err := a.WaitForExit()
			if err != nil {
				log.Errorf("Error exiting app: %v", err)
			}
		}
	})
}

func configureSystemTray(a *app.App) error {
	menu.enable = a.ShowUI
	if !menu.enable {
		return nil
	}

	iconTemplate := "%s_16.ico"
	if runtime.GOOS == "darwin" {
		iconTemplate = "%s_32.ico"
	}

	for _, name := range []string{"connected", "connectedalert", "disconnected", "disconnectedalert"} {
		icon, err := icons.Asset(fmt.Sprintf(iconTemplate, name))
		if err != nil {
			return fmt.Errorf("Unable to load %v icon for system tray: %v", name, err)
		}
		iconsByName[name] = icon
	}

	systray.SetTooltip("Lantern")
	menu.status = systray.AddMenuItem("", "")
	menu.status.Disable()
	menu.toggle = systray.AddMenuItem("", "")
	systray.AddSeparator()
	menu.upgrade = systray.AddMenuItem(i18n.T("TRAY_UPGRADE_TO_PRO"), i18n.T("TRAY_UPGRADE_TO_PRO"))
	menu.show = systray.AddMenuItem(i18n.T("TRAY_SHOW_LANTERN"), i18n.T("TRAY_SHOW_LANTERN"))
	systray.AddSeparator()
	menu.quit = systray.AddMenuItem(i18n.T("TRAY_QUIT"), i18n.T("TRAY_QUIT"))
	a.OnStatsChange(func(newStats stats.Stats) {
		menu.stMx.Lock()
		menu.st = newStats
		menu.stMx.Unlock()
		statsUpdated()
	})

	go func() {
		for {
			select {
			case <-menu.toggle.ClickedCh:
				menu.stMx.Lock()
				disconnected := menu.st.Disconnected
				menu.stMx.Unlock()
				if disconnected {
					a.Connect()
				} else {
					a.Disconnect()
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
	statsUpdated()
}

func statsUpdated() {
	menu.stMx.RLock()
	st := menu.st
	menu.stMx.RUnlock()

	iconName := "connected"
	statusKey := st.Status
	if st.Disconnected || !st.HasSucceedingProxy {
		iconName = "disconnected"
	}
	if st.HitDataCap && !st.IsPro {
		iconName += "alert"
		if !st.Disconnected {
			statusKey = "throttled"
		}
	}

	systray.SetIcon(iconsByName[iconName])
	status := i18n.T("TRAY_STATUS", i18n.T("status."+statusKey))
	menu.status.SetTitle(status)
	menu.status.SetTooltip(status)

	if st.IsPro {
		menu.upgrade.Hide()
	} else {
		menu.upgrade.Show()
	}

	if st.Disconnected {
		menu.toggle.SetTitle(i18n.T("TRAY_CONNECT"))
		menu.toggle.SetTooltip(i18n.T("TRAY_CONNECT"))
	} else {
		menu.toggle.SetTitle(i18n.T("TRAY_DISCONNECT"))
		menu.toggle.SetTooltip(i18n.T("TRAY_DISCONNECT"))
	}
}
