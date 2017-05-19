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
	ProxyAddr string
}

func (o *ConfigOpts) For() service.Type {
	return ServiceType
}

func (o *ConfigOpts) Complete() string {
	if o.ProxyAddr == "" {
		return "missing ProxyAddr"
	}
	return ""
}

type Sysproxy struct {
	mu          sync.Mutex
	on          bool
	initialized bool
	proxyAddr   string
}

func New() service.Impl {
	return &Sysproxy{}
}

func (p *Sysproxy) GetType() service.Type {
	return ServiceType
}

func (p *Sysproxy) Reconfigure(_ service.Publisher, opts service.ConfigOpts) {
	p.proxyAddr = opts.(*ConfigOpts).ProxyAddr
}

func (p *Sysproxy) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.initialized {
		err := setUpSysproxyTool()
		if err != nil {
			log.Error(err)
			return
		}
		p.initialized = true
	}
	doSysproxyOn(p.proxyAddr)
	p.on = true
}

func (p *Sysproxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	doSysproxyOff(p.proxyAddr)
	p.on = false
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
	log.Debugf("Unsetting lantern as system proxy at: %v", addr)
	err := sysproxy.Off(addr)
	if err != nil {
		op.FailIf(log.Errorf("Unable to unset lantern as system proxy: %v", err))
		return
	}
	log.Debug("Unset lantern as system proxy")
}
