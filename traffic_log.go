package flashlight

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/trafficlog"
	"github.com/getlantern/trafficlog-flashlight/tlproc"
)

const (
	trafficlogStartTimeout   = 5 * time.Second
	trafficlogRequestTimeout = time.Second

	// This message is only displayed when the traffic log needs to be installed.
	trafficlogInstallPrompt = "Lantern needs your permission to install diagnostic tools"

	// An asset in the icons package.
	trafficlogInstallIcon = "connected_32.ico"

	// This file, in the config directory, holds a timestamp from the last failed installation.
	trafficlogLastFailedInstallFile = "tl_last_failed"
)

// This should be run in an independent routine as it may need to install and block for a
// user-action granting permissions.
func (f *Flashlight) configureTrafficLog(cfg *config.Global) {
	f.trafficLogLock.Lock()
	f.proxiesLock.RLock()
	defer f.trafficLogLock.Unlock()
	defer f.proxiesLock.RUnlock()

	forceTrafficLog := f.flagsAsMap["force-traffic-log"].(bool)
	enableTrafficLog := false
	enableTrafficLog = f.featureEnabled(config.FeatureTrafficLog) || forceTrafficLog
	opts := new(config.TrafficLogOptions)
	if err := f.featureOptions(config.FeatureTrafficLog, opts); err != nil && !forceTrafficLog {
		log.Errorf("failed to unmarshal traffic log options: %v", err)
		return
	}
	if forceTrafficLog {
		// This flag is used in development to run the traffic log. We probably want to actually
		// capture some packets if this flag is set.
		if opts.CaptureBytes == 0 {
			opts.CaptureBytes = 10 * 1024 * 1024
		}
		if opts.SaveBytes == 0 {
			opts.SaveBytes = 10 * 1024 * 1024
		}
		// Use the most up-to-date binary in development.
		opts.Reinstall = true
		// Always try to install the traffic log in development.
		lastFailedPath, err := common.InConfigDir("", trafficlogLastFailedInstallFile)
		if err != nil {
			log.Debugf("Failed to create path to traffic log install-last-failed file: %v", err)
		} else if err := os.Remove(lastFailedPath); err != nil {
			log.Debugf("Failed to remove traffic log install-last-failed file: %v", err)
		}
	}

	switch {
	case enableTrafficLog && f.trafficLog == nil:
		installDir := appdir.General("Lantern")
		log.Debugf("Installing traffic log if necessary in %s", installDir)
		if err := f.tryTrafficLogInstall(installDir, *opts); err != nil {
			log.Errorf("Failed to install traffic log: %v", err)
			return
		}
		log.Debug("Turning traffic log on")
		if err := f.turnOnTrafficLog(installDir, *opts); err != nil {
			log.Errorf("Failed to turn on traffic log: %v", err)
		}

	case enableTrafficLog && f.trafficLog != nil:
		err := f.trafficLog.UpdateBufferSizes(opts.CaptureBytes, opts.SaveBytes)
		if err != nil {
			log.Debugf("Failed to update traffic log buffer sizes: %v", err)
		}

	case !enableTrafficLog && f.trafficLog != nil:
		log.Debug("Turning traffic log off")
		if err := f.trafficLog.Close(); err != nil {
			log.Errorf("Failed to close traffic log (this will create a memory leak): %v", err)
		}
		f.trafficLog = nil
	}
}

// Not concurrency-safe. Intended to serve as a helper to configureTrafficLog.
func (f *Flashlight) tryTrafficLogInstall(installDir string, opts config.TrafficLogOptions) error {
	u, err := user.Current()
	if err != nil {
		return errors.New("failed to look up current user for traffic log install: %v", err)
	}

	var iconFile string
	icon, err := icons.Asset(trafficlogInstallIcon)
	if err != nil {
		log.Debugf("Unable to load prompt icon during traffic log install: %v", err)
	} else {
		iconFile = filepath.Join(os.TempDir(), "lantern_tlinstall.ico")
		if err := ioutil.WriteFile(iconFile, icon, 0644); err != nil {
			// Failed to save the icon file, just use no icon.
			iconFile = ""
		}
	}

	lastFailedPath, err := common.InConfigDir("", trafficlogLastFailedInstallFile)
	if err != nil {
		return errors.New("failed to create path to traffic log install-last-failed file: %v", err)
	}
	lastFailedRaw, err := ioutil.ReadFile(lastFailedPath)
	if err != nil && !os.IsNotExist(err) {
		return errors.New("unable to open traffic log install-last-failed file: %v", err)
	}
	if lastFailedRaw != nil {
		if opts.WaitTimeSinceFailedInstall == 0 {
			return errors.New("aborting: install previously failed")
		}
		lastFailed := new(time.Time)
		if err := lastFailed.UnmarshalText(lastFailedRaw); err != nil {
			return errors.New("failed to parse traffic log install-last-failed file: %v", err)
		}
		if time.Since(*lastFailed) < opts.WaitTimeSinceFailedInstall {
			return errors.New(
				"aborting: last failed %v ago, wait time is %v",
				time.Since(*lastFailed), opts.WaitTimeSinceFailedInstall,
			)
		}
	}

	// Note that this is a no-op if the traffic log is already installed.
	installOpts := tlproc.InstallOptions{Overwrite: opts.Reinstall}
	installErr := tlproc.Install(
		installDir, u.Username, trafficlogInstallPrompt, iconFile, &installOpts)
	if installErr != nil {
		if b, err := time.Now().MarshalText(); err != nil {
			log.Errorf("Failed to marshal time for traffic log install-last-failed file: %v", err)
		} else if err := ioutil.WriteFile(lastFailedPath, b, 0644); err != nil {
			log.Errorf("Failed to write traffic log install-last-failed file: %v", err)
		}
		return errors.New("failed to install traffic log: %v", installErr)
	}
	return nil
}

// Not concurrency-safe. Intended to serve as a helper to configureTrafficLog.
func (f *Flashlight) turnOnTrafficLog(installDir string, opts config.TrafficLogOptions) error {
	var err error
	f.trafficLog, err = tlproc.New(
		opts.CaptureBytes,
		opts.SaveBytes,
		installDir,
		&tlproc.Options{
			Options: trafficlog.Options{
				MutatorFactory: new(trafficlog.AppStripperFactory),
			},
			StartTimeout:   trafficlogStartTimeout,
			RequestTimeout: trafficlogRequestTimeout,
		})
	if err != nil {
		return errors.Wrap(err)
	}
	// These goroutines will close when the traffic log is closed.
	go func() {
		for err := range f.trafficLog.Errors() {
			log.Debugf("Traffic log error: %v", err)
		}
	}()
	go func() {
		for stats := range f.trafficLog.Stats() {
			log.Debugf("Traffic log stats: %v", stats)
		}
	}()
	proxyAddrs := []string{}
	for _, p := range f.proxies {
		proxyAddrs = append(proxyAddrs, p.Addr())
	}
	if err := f.trafficLog.UpdateAddresses(proxyAddrs); err != nil {
		f.trafficLog.Close()
		f.trafficLog = nil
		return errors.New("failed to start traffic logging for proxies: %v", err)
	}
	return nil
}
