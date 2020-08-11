// +build !ios

package hellocap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"howett.net/plist"
)

// Just to get the tests to compile on Darwin
// TODO: delete me
var execPathRegexp = regexp.MustCompile(`"(.*)".*".*"`)

type lshandler struct {
	URLScheme string `plist:"LSHandlerURLScheme"`
	BundleID  string `plist:"LSHandlerRoleAll"`
}

type launchServicesDefaults struct {
	Handlers []lshandler `plist:"LSHandlers"`
}

type safari struct{}

func (s safari) name() string { return "Safari" }
func (s safari) close() error { return nil }

func (s safari) get(ctx context.Context, addr string) error {
	// TODO: implement me!
	// Safari does not support a headless mode. It may be possible to achieve something similar
	// using WebKit or even chromedp, but doing so in a reliable way without installing dependencies
	// on users' machines is non-trivial at the least.
	// TODO: capture a Safari hello and hard-code it somehow
	return nil
}

// If no default browser is explicitly configured, we assume Safari.
func defaultBrowser(ctx context.Context) (browser, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve current user information: %w", err)
	}

	launchServicesDomain := filepath.Join(
		u.HomeDir, "Library/Preferences/com.apple.LaunchServices/com.apple.launchservices.secure")
	if _, err = os.Stat(fmt.Sprintf("%s.plist", launchServicesDomain)); os.IsNotExist(err) {
		return safari{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat LaunchServices plist file: %w", err)
	}

	out, err := exec.CommandContext(ctx, "defaults", "read", launchServicesDomain).Output()
	if err != nil {
		return nil, wrapExecError("failed to read LaunchServices defaults", err)
	}

	defaults := new(launchServicesDefaults)
	if _, err = plist.Unmarshal(out, defaults); err != nil {
		return nil, fmt.Errorf("failed to parse LaunchServices defaults: %w", err)
	}

	var browserBundleID string
	for _, handler := range defaults.Handlers {
		// We prefer the https handler to the http handler.
		if handler.URLScheme == "https" {
			browserBundleID = handler.BundleID
		}
		if handler.URLScheme == "http" && browserBundleID == "" {
			browserBundleID = handler.BundleID
		}
	}
	if browserBundleID == "" {
		return safari{}, nil
	}
	return browserFromBundleID(ctx, browserBundleID)
}

func browserFromBundleID(ctx context.Context, bundleID string) (browser, error) {
	switch bundleID {
	case "com.google.chrome":
		bundle, err := appBundleFromID(ctx, "com.google.Chrome")
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		return chrome{filepath.Join(bundle, "Contents", "MacOS", "Google Chrome")}, nil

	case "com.apple.safari":
		// TODO: implement me!
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported bundle ID %s", bundleID)
	}
}

func appBundleFromID(ctx context.Context, bundleID string) (absPath string, err error) {
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
			return "", wrapExecError("mdfind failed", err)
		}
		if len(out) > 0 {
			return strings.Split(string(out), "\n")[0], nil
		}
	}
	return "", fmt.Errorf("could not find application for bundle ID %s", bundleID)
}
