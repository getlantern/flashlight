package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/getlantern/filepersist"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/icons"
)

var (
	isSysproxyOff = int32(0)
)

func setUpSysproxyTool() error {
	var iconFile string
	if runtime.GOOS == "darwin" {
		// We have to use a short filepath here because Cocoa won't display the
		// icon if the path is too long.
		iconFile = filepath.Join("/tmp", "escalatelantern.ico")
		icon, err := icons.Asset("icons/32on.ico")
		if err != nil {
			return fmt.Errorf("Unable to load escalation prompt icon: %v", err)
		}
		err = filepersist.Save(iconFile, icon, 0644)
		if err != nil {
			return fmt.Errorf("Unable to persist icon to disk: %v", err)
		}
		log.Debugf("Saved icon file to: %v", iconFile)
	}
	err := sysproxy.EnsureHelperToolPresent("sysproxy-cmd", "Lantern would like to be your system proxy", iconFile)
	if err != nil {
		return fmt.Errorf("Unable to set up sysproxy setting tool: %v", err)
	}
	return nil
}

func sysproxyOn() {
	doSysproxyOn()
	atomic.StoreInt32(&isSysproxyOff, 1)
}

func sysproxyOff() {
	if atomic.CompareAndSwapInt32(&isSysproxyOff, 1, 0) {
		doSysproxyOff()
	}
}

func doSysproxyOn() {
	addr, found := getProxyAddr()
	if !found {
		log.Errorf("Unable to set lantern as system proxy, no proxy address available")
		return
	}
	log.Debugf("Setting lantern as system proxy at: %v", addr)
	err := sysproxy.On(addr)
	if err != nil {
		log.Errorf("Unable to set lantern as system proxy: %v", err)
	}
}

func doSysproxyOff() {
	addr, found := getProxyAddr()
	if !found {
		log.Errorf("Unable to unset lantern as system proxy, no proxy address available")
		return
	}
	doSysproxyOffFor(addr)
}

func doSysproxyOffFor(addr string) {
	log.Debugf("Unsetting lantern as system proxy at: %v", addr)
	err := sysproxy.Off(addr)
	if err != nil {
		log.Errorf("Unable to unset lantern as system proxy: %v", err)
		return
	}
	log.Debug("Unset lantern as system proxy")
}

func getProxyAddr() (addr string, found bool) {
	var _addr interface{}
	_addr, found = client.Addr(5 * time.Minute)
	if found {
		addr = _addr.(string)
	}
	return
}
