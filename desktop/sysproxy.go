package desktop

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/filepersist"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/icons"

	log "github.com/sirupsen/logrus"
)

var (
	_sysproxyOff  func() error
	sysproxyOffMx sync.Mutex
)

func setUpSysproxyTool() error {
	var iconFile string
	if runtime.GOOS == "darwin" {
		icon, err := icons.Asset("connected_32.ico")
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
			log.Infof("Saved icon file to: %v", iconFile)
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
		log.Error(err)
		op.FailIf(err)
		return
	}
	log.Infof("Setting lantern as system proxy at: %v", addr)
	off, e := sysproxy.On(addr)
	if e != nil {
		err = errors.New("Unable to set lantern as system proxy: %v", e)
		log.Error(err)
		op.FailIf(err)
		return
	}
	sysproxyOffMx.Lock()
	_sysproxyOff = off
	sysproxyOffMx.Unlock()
	log.Info("Finished setting lantern as system proxy")
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
	log.Info("Force clearing system proxy directly, just in case")
	addr, found := getProxyAddr()
	if !found {
		foundErr := errors.New("Unable to find proxy address, can't force clear system proxy")
		log.Error(foundErr)
		op.FailIf(foundErr)
		return
	}
	doSysproxyClear(op, addr)
}

func doSysproxyOff(off func() error) {
	op := ops.Begin("sysproxy_off")
	defer op.End()
	log.Info("Unsetting lantern as system proxy using off function")
	err := off()
	if err != nil {
		unsetErr := errors.New("Unable to unset lantern as system proxy using off function: %v", err)
		log.Error(unsetErr)
		op.FailIf(unsetErr)
		return
	}
	log.Info("Unset lantern as system proxy using off function")
}

// clearSysproxyFor is like sysproxyOffFor, but records its activity under the
// sysproxy_clear op instead of the sysproxy_off op.
func clearSysproxyFor(addr string) {
	op := ops.Begin("sysproxy_clear")
	doSysproxyClear(op, addr)
	op.End()
}

func doSysproxyClear(op *ops.Op, addr string) {
	log.Infof("Clearing lantern as system proxy at: %v", addr)
	err := sysproxy.Off(addr)
	if err != nil {
		offErr := errors.New("Unable to clear lantern as system proxy: %v", err)
		log.Error(offErr)
		op.FailIf(offErr)
	} else {
		log.Info("Cleared lantern as system proxy")
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
