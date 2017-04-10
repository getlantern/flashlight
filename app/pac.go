package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/getlantern/filepersist"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/icons"
)

var (
	isPacOn   = int32(0)
	proxyAddr string
	cfgMutex  sync.RWMutex
)

func setUpPacTool(addr string) error {
	cfgMutex.Lock()
	proxyAddr = addr
	cfgMutex.Unlock()
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
		return fmt.Errorf("Unable to set up pac setting tool: %v", err)
	}
	return nil
}

func pacOn() {
	log.Debug("Setting lantern as system proxy")
	doPACOn()
	atomic.StoreInt32(&isPacOn, 1)
}

func pacOff() {
	if atomic.CompareAndSwapInt32(&isPacOn, 1, 0) {
		log.Debug("Unsetting lantern as system proxy")
		doPACOff()
	}
}

func doPACOn() {
	err := sysproxy.On(getProxyAddr())
	if err != nil {
		log.Errorf("Unable to set lantern as system proxy: %v", err)
	}
}

func doPACOff() {
	err := sysproxy.Off(getProxyAddr())
	if err != nil {
		log.Errorf("Unable to unset lantern as system proxy: %v", err)
		return
	}
	log.Debug("Unset lantern as system proxy")
}

func getProxyAddr() string {
	cfgMutex.RLock()
	addr := proxyAddr
	cfgMutex.RUnlock()
	return addr
}
