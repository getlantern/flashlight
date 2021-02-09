// Package trafficlog provides a log of network traffic, intended for use in the client.
package trafficlog

import (
	stderrors "errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"
	"github.com/getlantern/trafficlog"
	"github.com/getlantern/trafficlog-flashlight/tlproc"
	"github.com/getlantern/yaml"
)

const (
	startTimeout        = 5 * time.Second
	requestTimeout      = time.Second
	defaultSaveDuration = 5 * time.Minute

	// This file, in the config directory, holds information about installation failures.
	installFailuresFilename = "tl_install_failures.yaml"

	yamlableTimeFormat = time.RFC3339
)

var errTrafficLogDisabled = errors.New("traffic log is disabled")

type yamlableTime time.Time

func yamlableNow() *yamlableTime {
	yt := yamlableTime(time.Now())
	return &yt
}

func (yt *yamlableTime) GetYAML() (tag string, value interface{}) {
	if yt == nil {
		return "", time.Time(yamlableTime{}).Format(yamlableTimeFormat)
	}
	return "", time.Time(*yt).Format(yamlableTimeFormat)
}

func (yt *yamlableTime) SetYAML(tag string, value interface{}) bool {
	valueString, ok := value.(string)
	if !ok {
		return false
	}
	t, err := time.Parse(yamlableTimeFormat, valueString)
	if err != nil {
		return false
	}
	*yt = yamlableTime(t)
	return true
}

func (yt *yamlableTime) IsZero() bool {
	if yt == nil {
		return true
	}
	return time.Time(*yt).IsZero()
}

func (yt *yamlableTime) SetToZero() {
	*yt = yamlableTime{}
}

func (yt *yamlableTime) timeSince() time.Duration {
	return time.Since(time.Time(*yt))
}

// A YAML file in which we store data about traffic log installation failures.
type installFailuresFile struct {
	LastFailed, LastDenial *yamlableTime
	Denials                int

	path string
}

// If the file does not exist, this function returns a default value, pointed at the input path.
func openInstallFailuresFile(path string) (*installFailuresFile, error) {
	f := new(installFailuresFile)
	b, err := ioutil.ReadFile(path)
	if stderrors.Is(err, os.ErrNotExist) {
		return &installFailuresFile{&yamlableTime{}, &yamlableTime{}, 0, path}, nil
	} else if err != nil {
		return nil, errors.New("failed to read file: %v", err)
	}
	if err := yaml.Unmarshal(b, f); err != nil {
		return nil, errors.New("failed to unmarshal file: %v", err)
	}
	f.path = path
	return f, nil
}

// Write changes to disk.
func (f installFailuresFile) flushChanges() error {
	b, err := yaml.Marshal(f)
	if err != nil {
		return errors.New("failed to marshal: %v", err)
	}
	if err := ioutil.WriteFile(f.path, b, 0644); err != nil {
		return errors.New("failed to write file: %v", err)
	}
	return nil
}

func trafficlogInstallPrompt() string {
	translatedAppName := i18n.T(strings.ToUpper(common.AppName))
	return i18n.T("BACKEND_INSTALL_DIAGNOSTIC_TOOLS", translatedAppName, translatedAppName)
}

var log = golog.LoggerFor("trafficlog")

// A PcapngRequest is used to instruct the traffic log to write all packets in the save buffer to
// the specified writer.
type PcapngRequest struct {
	W io.Writer

	// Error is used to communicate whether an error occurred saving the capture. A value is always
	// sent on this channel; nil is sent on success. Ignoring this channel will create a goroutine
	// leak. If communication is not needed, this channel should be nil.
	Error chan<- error
}

// ConfigUpdate is used to update the traffic log when new configuration is received.
type ConfigUpdate struct {
	Enabled    bool
	Config     config.Global
	InstallDir string
}

// A TrafficLog intended for use in the client.
type TrafficLog struct {
	// proc is nil when the traffic log is disabled
	proc *tlproc.TrafficLogProcess

	pcapRequestChan chan PcapngRequest
	currentProxies  []balancer.Dialer
	opts            config.TrafficLogOptions

	// See ForceEnable().
	forceEnable bool

	lock sync.Mutex
}

// New TrafficLog.
func New(proxiesChan <-chan []balancer.Dialer, configChan <-chan ConfigUpdate) *TrafficLog {
	tl := &TrafficLog{
		pcapRequestChan: make(chan PcapngRequest),
	}
	go func() {
		for cfg := range configChan {
			tl.handleConfigUpdate(cfg)
		}
	}()
	go func() {
		for proxies := range proxiesChan {
			tl.handleProxiesUpdate(proxies)
		}
	}()
	go func() {
		for req := range tl.pcapRequestChan {
			go func(r PcapngRequest) {
				if r.Error != nil {
					r.Error <- tl.handlePcapRequest(r)
				} else {
					tl.handlePcapRequest(r)
				}
			}(req)
		}
	}()
	return tl
}

// PcapngRequestChannel is used to instruct the traffic log that captured packets should be saved to
// the traffic log's save buffer.
func (tl *TrafficLog) PcapngRequestChannel() chan<- PcapngRequest {
	return tl.pcapRequestChan
}

// ForceEnable is used to force the traffic log to always run. If no config is provided, default
// config will be used. This method is intended for development.
func (tl *TrafficLog) ForceEnable(installDir string) {
	tl.lock.Lock()
	defer tl.lock.Unlock()
	tl.forceEnable = true
	if tl.proc == nil {
		log.Debug("Starting new traffic log process")
		if err := tl.installAndStart(installDir); err != nil {
			log.Errorf("Failed to start traffic log process: %v", err)
		}
	}
}

func (tl *TrafficLog) handleConfigUpdate(update ConfigUpdate) {
	tl.lock.Lock()
	defer tl.lock.Unlock()

	if tl.forceEnable {
		update.Enabled = true
	}

	if err := update.Config.UnmarshalFeatureOptions(config.FeatureTrafficLog, &tl.opts); err != nil {
		if !tl.forceEnable {
			log.Errorf("failed to unmarshal traffic log options: %v", err)
			return
		}
		log.Debugf("setting default traffic log options; failed to unmarshal from config: %v", err)
		tl.opts = config.TrafficLogOptions{
			CaptureBytes:               10 * 1024 * 1024,
			SaveBytes:                  10 * 1024 * 1024,
			CaptureSaveDuration:        5 * time.Minute,
			Reinstall:                  true,
			WaitTimeSinceFailedInstall: 1,
			UserDenialThreshold:        3,
			TimeBeforeDenialReset:      24 * time.Hour,
		}
	}
	if tl.forceEnable {
		failuresPath := filepath.Join(update.InstallDir, installFailuresFilename)
		if err := os.Remove(failuresPath); err != nil {
			log.Debugf("Failed to remove traffic log install-last-failed file: %v", err)
		}
	}

	switch {
	case update.Enabled && tl.proc != nil:
		err := tl.proc.UpdateBufferSizes(tl.opts.CaptureBytes, tl.opts.SaveBytes)
		if err != nil {
			log.Debugf("Failed to update traffic log buffer sizes: %v", err)
		}

	case update.Enabled && tl.proc == nil:
		log.Debug("Starting new traffic log process")
		if err := tl.installAndStart(update.InstallDir); err != nil {
			log.Errorf("Failed to start traffic log process: %v", err)
		}

	case !update.Enabled && tl.proc != nil:
		log.Debug("Stopping traffic log process")
		if err := tl.proc.Close(); err != nil {
			log.Errorf("Failed to close traffic log (this may create a memory leak): %v", err)
		}
		tl.proc = nil

	}
}

func (tl *TrafficLog) installAndStart(installDir string) error {
	err := install(installDir, tl.opts)
	if err != nil {
		return errors.New("install failed (install dir: '%s'): %v", installDir, err)
	}

	tl.proc, err = tlproc.New(
		tl.opts.CaptureBytes,
		tl.opts.SaveBytes,
		installDir,
		&tlproc.Options{
			Options: trafficlog.Options{
				MutatorFactory: new(trafficlog.AppStripperFactory),
			},
			StartTimeout:   startTimeout,
			RequestTimeout: requestTimeout,
		})
	if err != nil {
		return errors.Wrap(err)
	}

	// These goroutines will close when the process is closed.
	go func() {
		for err := range tl.proc.Errors() {
			log.Debugf("Traffic log error: %v", err)
		}
	}()
	go func() {
		for stats := range tl.proc.Stats() {
			log.Debugf("Traffic log stats: %v", stats)
		}
	}()

	proxyAddrs := make([]string, len(tl.currentProxies))
	for i, p := range tl.currentProxies {
		proxyAddrs[i] = p.Addr()
	}
	if err := tl.proc.UpdateAddresses(proxyAddrs); err != nil {
		tl.proc.Close()
		tl.proc = nil
		return errors.New("failed to start traffic logging for proxies: %v", err)
	}
	return nil
}

func (tl *TrafficLog) handleProxiesUpdate(proxies []balancer.Dialer) {
	tl.lock.Lock()
	defer tl.lock.Unlock()
	tl.currentProxies = proxies
}

func (tl *TrafficLog) handlePcapRequest(req PcapngRequest) error {
	tl.lock.Lock()
	defer tl.lock.Unlock()

	if tl.proc == nil {
		return errTrafficLogDisabled
	}
	for _, p := range tl.currentProxies {
		if err := tl.proc.SaveCaptures(p.Addr(), tl.opts.CaptureSaveDuration); err != nil {
			return errors.New("failed to save captures for %s: %v", p.Name(), err)
		}
	}
	if err := tl.proc.WritePcapng(req.W); err != nil {
		return errors.New("failed to write saved packets: %v", err)
	}
	return nil
}

func install(installDir string, opts config.TrafficLogOptions) error {
	u, err := user.Current()
	if err != nil {
		return errors.New("failed to look up current user for traffic log install: %v", err)
	}

	var iconFile string
	icon, err := icons.Asset(appIcon("connected"))
	if err != nil {
		log.Debugf("Unable to load prompt icon during traffic log install: %v", err)
	} else {
		iconFile = filepath.Join(os.TempDir(), "lantern_tlinstall.ico")
		if err := ioutil.WriteFile(iconFile, icon, 0644); err != nil {
			// Failed to save the icon file, just use no icon.
			iconFile = ""
		}
	}

	failuresFilePath := filepath.Join(installDir, installFailuresFilename)
	failuresFile, err := openInstallFailuresFile(failuresFilePath)
	if err != nil {
		return errors.New("unable to open traffic log install-failures file: %v", err)
	}
	defer func() { failuresFile.flushChanges() }()
	if !failuresFile.LastDenial.IsZero() &&
		failuresFile.LastDenial.timeSince() > opts.TimeBeforeDenialReset {
		failuresFile.Denials = 0
		failuresFile.LastDenial.SetToZero()
	}
	if failuresFile.Denials >= opts.UserDenialThreshold {
		return errors.New(
			"user denials (%d) already meets threshold (%d)",
			failuresFile.Denials, opts.UserDenialThreshold,
		)
	}
	if !failuresFile.LastFailed.IsZero() {
		if opts.WaitTimeSinceFailedInstall == 0 {
			return errors.New("aborting: install previously failed")
		}
		if failuresFile.LastFailed.timeSince() < opts.WaitTimeSinceFailedInstall {
			return errors.New(
				"aborting: last failed %v ago, wait time is %v",
				failuresFile.LastFailed.timeSince(), opts.WaitTimeSinceFailedInstall,
			)
		}
	}

	// Note that this is a no-op if the traffic log is already installed.
	installOpts := tlproc.InstallOptions{Overwrite: opts.Reinstall}
	err = tlproc.Install(installDir, u.Username, trafficlogInstallPrompt(), iconFile, &installOpts)
	if err != nil {
		failuresFile.LastFailed = yamlableNow()
		if stderrors.Is(err, tlproc.ErrPermissionDenied) {
			failuresFile.Denials++
			failuresFile.LastDenial = yamlableNow()
		}
		return errors.Wrap(err)
	}
	return nil
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
	return "%s_32.ico"
}
