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
	"github.com/getlantern/golog"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/icons"
)

type systemproxy struct {
	isSysproxyOn int32
	log          golog.Logger
}

// newSystemProxy creates a new systemproxy
func newSystemProxy() *systemproxy {
	return &systemproxy{log: golog.LoggerFor("flashlight.app.sysproxy")}
}

func (sp *systemproxy) setUpSysproxyTool() error {
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
		sp.log.Debugf("Saved icon file to: %v", iconFile)
	}
	err := sysproxy.EnsureHelperToolPresent("sysproxy-cmd", "Lantern would like to be your system proxy", iconFile)
	if err != nil {
		return fmt.Errorf("Unable to set up sysproxy setting tool: %v", err)
	}
	return nil
}

func (sp *systemproxy) sysproxyOn() {
	sp.doSysproxyOn()
	atomic.StoreInt32(&sp.isSysproxyOn, 1)
}

func (sp *systemproxy) sysproxyOff() {
	if atomic.CompareAndSwapInt32(&sp.isSysproxyOn, 1, 0) {
		sp.doSysproxyOff()
	}
}

func (sp *systemproxy) doSysproxyOn() {
	op := ops.Begin("sysproxy_on")
	defer op.End()
	addr, found := sp.getProxyAddr()
	if !found {
		op.FailIf(sp.log.Errorf("Unable to set lantern as system proxy, no proxy address available"))
		return
	}
	sp.log.Debugf("Setting lantern as system proxy at: %v", addr)
	err := sysproxy.On(addr)
	if err != nil {
		op.FailIf(sp.log.Errorf("Unable to set lantern as system proxy: %v", err))
		return
	}
	sp.log.Debug("Finished setting lantern as system proxy")
}

func (sp *systemproxy) doSysproxyOff() {
	addr, found := sp.getProxyAddr()
	if !found {
		sp.log.Errorf("Unable to unset lantern as system proxy, no proxy address available")
		return
	}
	sp.doSysproxyOffFor(addr)
}

func (sp *systemproxy) doSysproxyOffFor(addr string) {
	op := ops.Begin("sysproxy_off")
	defer op.End()
	sp.log.Debugf("Unsetting lantern as system proxy at: %v", addr)
	err := sysproxy.Off(addr)
	if err != nil {
		op.FailIf(sp.log.Errorf("Unable to unset lantern as system proxy: %v", err))
		return
	}
	sp.log.Debug("Unset lantern as system proxy")
}

func (sp *systemproxy) getProxyAddr() (addr string, found bool) {
	var _addr interface{}
	_addr, found = client.Addr(5 * time.Minute)
	if found {
		addr = _addr.(string)
	}
	return
}
