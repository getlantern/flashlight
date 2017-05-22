package sysproxy

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/getlantern/filepersist"
	"github.com/getlantern/golog"
	"github.com/getlantern/sysproxy"

	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/service"
)

var (
	ServiceType service.Type = "app.sysproxy"
	log                      = golog.LoggerFor("app.sysproxy")
)

type ConfigOpts struct {
	// Enable is the only dynamic config of the sysproxy service. It turns on/off
	// system's proxy setting accordingly.
	Enable bool
}

func (o *ConfigOpts) For() service.Type {
	return ServiceType
}

func (o *ConfigOpts) Complete() string {
	return ""
}

type Sysproxy struct {
	proxyAddr string

	mu     sync.Mutex
	enable bool
	on     bool
}

func New(proxyAddr string) *Sysproxy {
	err := setUpSysproxyTool()
	if err != nil {
		log.Error(err) // report once and do nothing else
	}
	log.Debugf("Create sysproxy service with proxy address %v", proxyAddr)
	return &Sysproxy{proxyAddr: proxyAddr}
}

func (p *Sysproxy) GetType() service.Type {
	return ServiceType
}

func (p *Sysproxy) Configure(opts service.ConfigOpts) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enable = opts.(*ConfigOpts).Enable
	p.do(p.enable)
}

func (p *Sysproxy) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.do(p.enable)
}

// Stop attempts to turn off Lantern as the system proxy and records
// the success/failure as the sysproxy_off op.
func (p *Sysproxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enable = false
	p.do(false)
}

// Clear clears the system proxy setting regardless if it was started by this
// instance, and records its activity under the sysproxy_clear op instead of
// the sysproxy_off op.
func (p *Sysproxy) Clear() {
	op := ops.Begin("sysproxy_clear")
	defer op.End()
	op.FailIf(doSysproxyOffFor(p.proxyAddr))
}

func (p *Sysproxy) do(turnOn bool) {
	if turnOn && !p.on {
		doSysproxyOn(p.proxyAddr)
		p.on = true
	} else if !turnOn && p.on {
		doSysproxyOff(p.proxyAddr)
		p.on = false
	}
}

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

func doSysproxyOn(addr string) {
	op := ops.Begin("sysproxy_on")
	defer op.End()
	log.Debugf("Setting lantern as system proxy at: %v", addr)
	err := sysproxy.On(addr)
	if err != nil {
		op.FailIf(log.Errorf("Unable to set lantern as system proxy: %v", err))
		return
	}
	log.Debug("Finished setting lantern as system proxy")
}

func doSysproxyOff(addr string) {
	op := ops.Begin("sysproxy_off")
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
