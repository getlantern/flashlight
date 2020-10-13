package desktop

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/filepersist"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/ops"
)

var (
	_sysproxyOff  func() error
	sysproxyOffMx sync.Mutex
)

func setUpSysproxyTool() error {
	var iconFile string
	if common.Platform == "darwin" {
		icon, err := icons.Asset(appIcon("connected"))
		if err != nil {
			return fmt.Errorf("Unable to load escalation prompt icon: %v", err)
		}
		// We have to use a short filepath here because Cocoa won't display the
		// icon if the path is too long.
		iconFile = filepath.Join("/tmp", appIcon("escalate"))
		err = filepersist.Save(iconFile, icon, 0644)
		if err != nil {
			log.Errorf("Unable to persist icon to disk, fallback to default icon: %v", err)
		} else {
			log.Debugf("Saved icon file to: %v", iconFile)
		}
	}
	err := sysproxy.EnsureHelperToolPresent("sysproxy-cmd", "Lantern would like to be your system proxy", iconFile)
	if err != nil {
		return fmt.Errorf("Unable to set up sysproxy setting tool: %v", err)
	}
	return nil
}

func sysproxyOn() (err error) {
	op := ops.Begin("sysproxy_on")
	defer op.End()
	addr, found := getProxyAddr()
	if !found {
		err = errors.New("Unable to set lantern as system proxy, no proxy address available")
		op.FailIf(log.Error(err))
		return
	}
	log.Debugf("Setting lantern as system proxy at: %v", addr)
	off, e := sysproxy.On(addr)
	if e != nil {
		err = errors.New("Unable to set lantern as system proxy: %v", e)
		op.FailIf(log.Error(err))
		return
	}
	sysproxyOffMx.Lock()
	_sysproxyOff = off
	sysproxyOffMx.Unlock()
	log.Debug("Finished setting lantern as system proxy")
	return
}

func sysproxyOff() {
	sysproxyOffMx.Lock()
	off := _sysproxyOff
	_sysproxyOff = nil
	sysproxyOffMx.Unlock()

	if off != nil {
		doSysproxyOff(off)
	}

	op := ops.Begin("sysproxy_off_force")
	defer op.End()
	log.Debug("Force clearing system proxy directly, just in case")
	addr, found := getProxyAddr()
	if !found {
		op.FailIf(log.Error("Unable to find proxy address, can't force clear system proxy"))
		return
	}
	doSysproxyClear(op, addr)
}

func doSysproxyOff(off func() error) {
	op := ops.Begin("sysproxy_off")
	defer op.End()
	log.Debug("Unsetting lantern as system proxy using off function")
	err := off()
	if err != nil {
		op.FailIf(log.Errorf("Unable to unset lantern as system proxy using off function: %v", err))
		return
	}
	log.Debug("Unset lantern as system proxy using off function")
}

// clearSysproxyFor is like sysproxyOffFor, but records its activity under the
// sysproxy_clear op instead of the sysproxy_off op.
func clearSysproxyFor(addr string) {
	op := ops.Begin("sysproxy_clear")
	doSysproxyClear(op, addr)
	op.End()
}

func doSysproxyClear(op *ops.Op, addr string) {
	log.Debugf("Clearing lantern as system proxy at: %v", addr)
	err := sysproxy.Off(addr)
	if err != nil {
		op.FailIf(log.Errorf("Unable to clear lantern as system proxy: %v", err))
	} else {
		log.Debug("Cleared lantern as system proxy")
	}
}

func getProxyAddr() (addr string, found bool) {
	var _addr interface{}
	_addr, found = client.Addr(5 * time.Minute)
	if found {
		addr = _addr.(string)
	}
	return
}

func appIcon(name string) string {
	return strings.ToLower(common.AppName) + "_" + fmt.Sprintf(iconTemplate(), name)
}

func iconTemplate() string {
	if common.Platform == "darwin" {
		if common.AppName == "Beam" {
			return "%s_32.png"
		}
		// Lantern doesn't have png files to support dark mode yet
		return "%s_32.ico"
	}
	return "%s_16.ico"
}
