// +build !ios

package browsers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/getlantern/flashlight/ops"
	"howett.net/plist"
)

// ErrorUnknownBundleID means that an unknown bundle ID was encountered. However, this bundle ID may
// still be useful.
type ErrorUnknownBundleID string

func (err ErrorUnknownBundleID) Error() string {
	return fmt.Sprintf("unknown bundle ID '%s'", string(err))
}

// BundleID provides the unrecognized bundle ID underlying this error. This function is provided for
// convenience and readability; it is equivalent to string(err).
func (err ErrorUnknownBundleID) BundleID() string {
	return string(err)
}

// Possible Browsers.
const (
	Unknown Browser = iota
	Chrome
	Firefox
	Edge
	Safari
)

func (b Browser) String() string {
	switch b {
	case Chrome:
		return "Google Chrome"
	case Firefox:
		return "Mozilla Firefox"
	case Edge:
		return "Microsoft Edge"
	case Safari:
		return "Apple Safari"
	default:
		return "Unknown"
	}
}

// AppBundle returns the absolute path to the application bundle for this browser.
func (b Browser) AppBundle(ctx context.Context) (string, error) {
	var bundleID string
	switch b {
	case Chrome:
		bundleID = "com.google.Chrome"
	case Firefox:
		bundleID = "org.mozilla.firefox"
	case Edge:
		bundleID = "com.microsoft.edgemac"
	case Safari:
		bundleID = "com.apple.Safari"
	default:
		return "", ErrUnsupportedAction
	}

	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("unable to retrieve current user information: %w", err)
	}
	for _, appDir := range []string{
		filepath.Join(u.HomeDir, "Applications"),
		"/Applications",
	} {
		cmd := exec.CommandContext(
			ctx, "mdfind", "-onlyin", appDir, "kMDItemCFBundleIdentifier", "=", bundleID)
		out, err := cmd.Output()
		if err != nil {
			// mdfind exits 0 if no bundle was found, so this would be a failure to run the command.
			return "", wrapExecError("mdfind failed", err)
		}
		if len(out) > 0 {
			return strings.Split(string(out), "\n")[0], nil
		}
	}
	return "", errors.New("could not find application bundle")
}

// SystemDefault returns the default web browser. Specifically, this is the default launch
// service for HTTPS or HTTP links. May return ErrorUnknownBundleID.
func SystemDefault(ctx context.Context) (Browser, error) {
	op := ops.Begin("")
	op.Set("os", "darwin")
	b, err := systemDefault(ctx)
	op.FailIf(err)
	op.End()
	return b, err
}

func systemDefault(ctx context.Context) (Browser, error) {
	u, err := user.Current()
	if err != nil {
		return Unknown, fmt.Errorf("unable to retrieve current user information: %w", err)
	}

	launchServicesDomain := filepath.Join(
		u.HomeDir, "Library/Preferences/com.apple.LaunchServices/com.apple.launchservices.secure")
	if _, err = os.Stat(fmt.Sprintf("%s.plist", launchServicesDomain)); os.IsNotExist(err) {
		return Safari, nil
	} else if err != nil {
		return Unknown, fmt.Errorf("failed to stat LaunchServices plist file: %w", err)
	}

	out, err := exec.CommandContext(ctx, "defaults", "read", launchServicesDomain).Output()
	if err != nil {
		return Unknown, wrapExecError("failed to read LaunchServices defaults", err)
	}

	defaults := new(launchServicesDefaults)
	if _, err = plist.Unmarshal(out, defaults); err != nil {
		return Unknown, fmt.Errorf("failed to parse LaunchServices defaults: %w", err)
	}

	var bundleID string
	for _, handler := range defaults.Handlers {
		// We prefer the https handler to the http handler.
		if handler.URLScheme == "https" {
			bundleID = handler.BundleID
		}
		if handler.URLScheme == "http" && bundleID == "" {
			bundleID = handler.BundleID
		}
	}
	switch bundleID {
	case "com.google.chrome":
		return Chrome, nil
	case "org.mozilla.firefox":
		return Firefox, nil
	case "com.microsoft.edgemac":
		return Edge, nil
	case "com.apple.safari", "":
		return Safari, nil
	default:
		return Unknown, ErrorUnknownBundleID(bundleID)
	}
}

type lshandler struct {
	URLScheme string `plist:"LSHandlerURLScheme"`
	BundleID  string `plist:"LSHandlerRoleAll"`
}

type launchServicesDefaults struct {
	Handlers []lshandler `plist:"LSHandlers"`
}
