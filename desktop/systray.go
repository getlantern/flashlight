// +build !headless

package desktop

import (
	"fmt"
	"strings"
	"sync"

	"github.com/getlantern/i18n"
	"github.com/getlantern/systray"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/stats"
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
	iconsByName        = make(map[string][]byte)
	translationAppName = strings.ToUpper(common.AppName)
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

func RunOnSystrayReady(standalone bool, a systrayCallbacks, onReady func()) {
	onExit := func() {
		if a.Exit(nil) {
			err := a.WaitForExit()
			if err != nil {
				log.Errorf("Error exiting app: %v", err)
			}
		}
	}

	if standalone {
		log.Error("Standalone mode currently not supported, opening in system browser")
		// TODO: re-enable standalone mode when systray library has been stabilized
		// systray.RunWithAppWindow(i18n.T("TRAY_LANTERN"), 1024, 768, onReady, onExit)
		// } else {
	}
	systray.Run(onReady, onExit)
}

func QuitSystray(a *App) {
	// Typically, systray.Quit will actually be what causes the app to exit, but
	// in the case of an uncaught Fatal error or CTRL-C, the app will exit before the
	// systray and we need it to call systray.Quit().
	if a.ShouldShowUI() {
		systray.Quit()
	} else {
		a.Exit(nil)
	}
}

func configureSystemTray(a systrayCallbacks) error {
	menu.enable = a.ShouldShowUI()
	if !menu.enable {
		return nil
	}

	iconTemplate := "%s_16.ico"
	if common.Platform == "darwin" {
		iconTemplate = "%s_32.ico"
	}

	for _, name := range []string{"connected", "connectedalert", "disconnected", "disconnectedalert"} {
		name = appIcon(name)
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

	// Suppress showing "Update to Pro" until user status is got from pro-server.
	if common.ProAvailable {
		menu.st.IsPro = true
	} else {
		menu.upgrade.Hide()
	}

	menu.st.Status = stats.STATUS_CONNECTING
	statsUpdated()
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
	if _, err := i18n.SetLocale(language); err != nil {
		log.Errorf("i18n.SetLocale(%s) failed: %q", language, err)
		return
	}
	refreshMenuItems()
	statsUpdated()
}

func refreshMenuItems() {
	systray.SetTooltip(i18n.T(translationAppName))
	menu.upgrade.SetTitle(i18n.T("TRAY_UPGRADE_TO_PRO"))
	menu.show.SetTitle(i18n.T("TRAY_SHOW", i18n.T(translationAppName)))
	menu.quit.SetTitle(i18n.T("TRAY_QUIT", i18n.T(translationAppName)))
}

func appIcon(name string) string {
	return strings.ToLower(common.AppName) + "_" + name
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

	iconName = appIcon(iconName)
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

	if st.IsPro || !common.ProAvailable {
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
