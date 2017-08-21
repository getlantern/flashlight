package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/getlantern/filepersist"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/icons"
)

var (
	isSysproxyOn = int32(0)
)

func setUpSysproxyTool() error {
	var iconFile string
	if runtime.GOOS == "darwin" {
		icon, err := icons.Asset("icons/32on.ico")
		if err != nil {
			return fmt.Errorf("Unable to load escalation prompt icon: %v", err)
		}
		// We have to use a short filepath here because Cocoa won't display the
		// icon if the path is too long.
		iconFile = filepath.Join("/tmp", "escalatelantern.ico")
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

func sysproxyOn() {
	doSysproxyOn()
	atomic.StoreInt32(&isSysproxyOn, 1)
}

func sysproxyOff() {
	if atomic.CompareAndSwapInt32(&isSysproxyOn, 1, 0) {
		doSysproxyOff()
	}
}

func doSysproxyOn() {
	op := ops.Begin("sysproxy_on")
	defer op.End()
	addr, found := getProxyAddr()
	if !found {
		op.FailIf(log.Errorf("Unable to set lantern as system proxy, no proxy address available"))
		return
	}
	log.Debugf("Setting lantern as system proxy at: %v", addr)
	err := sysproxy.On(addr)
	if err != nil {
		op.FailIf(log.Errorf("Unable to set lantern as system proxy: %v", err))
		return
	}
	log.Debug("Finished setting lantern as system proxy")
}

func doSysproxyOff() {
	addr, found := getProxyAddr()
	if !found {
		log.Errorf("Unable to unset lantern as system proxy, no proxy address available")
		return
	}
	sysproxyOffFor(addr)
}

// sysproxyOffFor attempts to turn off Lantern as the system proxy and records
// the success/failure as the sysproxy_off op.
func sysproxyOffFor(addr string) {
	op := ops.Begin("sysproxy_off")
	defer op.End()
	op.FailIf(doSysproxyOffFor(addr))
}

// clearSysproxyFor is like sysproxyOffFor, but records its activity under the
// sysproxy_clear op instead of the sysproxy_off op.
func clearSysproxyFor(addr string) {
	op := ops.Begin("sysproxy_clear")
	defer op.End()
	op.FailIf(doSysproxyOffFor(addr))
}

func doSysproxyOffFor(addr string) error {
	log.Debugf("Unsetting lantern as system proxy at: %v", addr)
	err := sysproxy.Off(addr)
	if err != nil {
		return log.Errorf("Unable to unset lantern as system proxy: %v", err)
	}
	log.Debug("Unset lantern as system proxy")
	return nil
}

func getProxyAddr() (addr string, found bool) {
	var _addr interface{}
	_addr, found = client.Addr(5 * time.Minute)
	if found {
		addr = _addr.(string)
	}
	return
}
