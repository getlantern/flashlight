package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/filepersist"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/icons"
)

var (
	isSysproxyOn = int32(0)
	cfgMutex     sync.RWMutex
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
	err := sysproxy.EnsureHelperToolPresent("pac-cmd", "Lantern would like to be your system proxy", iconFile)
	if err != nil {
		return fmt.Errorf("Unable to set up pac setting tool: %v", err)
	}
	return nil
}

func sysproxyOn() {
	log.Debug("Setting lantern as system proxy")
	err := sysproxy.On(getHTTPAddr())
	if err != nil {
		log.Errorf("Unable to set lantern as system proxy: %v", err)
	}
	atomic.StoreInt32(&isSysproxyOn, 1)
}

func sysproxyOff() {
	if atomic.CompareAndSwapInt32(&isSysproxyOn, 1, 0) {
		log.Debug("Unsetting lantern as system proxy")
		doSysproxyOff(getHTTPAddr())
		log.Debug("Unset lantern as system proxy")
	}
}

func doSysproxyOff(addr string) {
	err := sysproxy.Off(addr)
	if err != nil {
		log.Errorf("Unable to unset lantern as system proxy: %v", err)
	}
}

func getHTTPAddr() string {
	_httpAddr, _ := client.Addr(1 * time.Minute)
	return _httpAddr.(string)
}
