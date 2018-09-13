// +build !headless

package main

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/stats"

	log "github.com/sirupsen/logrus"
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

type systrayCallbacks interface {
	WaitForExit() error
	AddExitFunc(string, func())
	Exit(error) bool
	ShouldShowUI() bool
	OnTrayShow()
	OnTrayUpgrade()
	Connect()
	Disconnect()
	OnStatsChange(func(stats.Stats))
}

func runOnSystrayReady(a systrayCallbacks, f func()) {
	// Typically, systray.Quit will actually be what causes the app to exit, but
	// in the case of an uncaught Fatal error, the app will exit before the
	// systray and we need it to call systray.Quit().
	a.AddExitFunc("quitting systray", systray.Quit)

	systray.Run(f, func() {
		if a.Exit(nil) {
			err := a.WaitForExit()
			if err != nil {
				log.Errorf("Error exiting app: %v", err)
			}
		}
	})
}

func configureSystemTray(a systrayCallbacks) error {
	menu.enable = a.ShouldShowUI()
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

	menu.status = systray.AddMenuItem("", "")
	menu.status.Disable()
	menu.toggle = systray.AddMenuItem("", "")
	systray.AddSeparator()
	menu.upgrade = systray.AddMenuItem("", "")
	menu.show = systray.AddMenuItem("", "")
	systray.AddSeparator()
	menu.quit = systray.AddMenuItem("", "")
	refreshMenuItems()
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
				a.OnTrayShow()
			case <-menu.upgrade.ClickedCh:
				a.OnTrayUpgrade()
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
	refreshMenuItems()
	statsUpdated()
}

func refreshMenuItems() {
	systray.SetTooltip(i18n.T("TRAY_LANTERN"))
	menu.upgrade.SetTitle(i18n.T("TRAY_UPGRADE_TO_PRO"))
	menu.show.SetTitle(i18n.T("TRAY_SHOW_LANTERN"))
	menu.quit.SetTitle(i18n.T("TRAY_QUIT"))
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
	} else if len(st.Alerts) > 0 {
		iconName += "alert"
		// Show the first one as status if there are multiple alerts
		statusKey = st.Alerts[0].Alert()
	}

	systray.SetIcon(iconsByName[iconName])
	status := i18n.T("TRAY_STATUS", i18n.T("status."+statusKey))
	menu.status.SetTitle(status)

	if st.IsPro {
		menu.upgrade.Hide()
	} else {
		menu.upgrade.Show()
	}

	if st.Disconnected {
		menu.toggle.SetTitle(i18n.T("TRAY_CONNECT"))
	} else {
		menu.toggle.SetTitle(i18n.T("TRAY_DISCONNECT"))
	}
}
