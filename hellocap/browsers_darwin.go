// +build !ios

package hellocap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/getlantern/flashlight/browsers"
)

func defaultBrowser(ctx context.Context) (browser, error) {
	_browser, err := browsers.SystemDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain default browser: %w", err)
	}
	bundle, err := _browser.AppBundle(ctx)
	if errors.Is(err, browsers.ErrUnsupportedAction) {
		return nil, fmt.Errorf("unsupported browser '%v'", _browser)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve app bundle for '%v': %w", _browser, err)
	}

	switch _browser {
	case browsers.Chrome:
		return chrome{filepath.Join(bundle, "Contents", "MacOS", "Google Chrome")}, nil

	case browsers.Firefox:
		f, err := newFirefoxInstance(filepath.Join(bundle, "Contents", "MacOS", "firefox"))
		if err != nil {
			return nil, fmt.Errorf("failed to create firefox instance: %w", err)
		}
		return f, nil

	case browsers.Edge:
		return edgeChromium{filepath.Join(bundle, "Contents", "MacOS", "Microsoft Edge")}, nil

	default:
		return nil, fmt.Errorf("unsupported browser '%v'", _browser)
	}
}

type firefox struct {
	path, profileDirectory string
}

func newFirefoxInstance(path string) (*firefox, error) {
	pDir, err := newFirefoxProfileDirectory()
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary Firefox profile: %w", err)
	}
	return &firefox{path, pDir}, nil
}

func (f firefox) name() string { return "Mozilla Firefox" }

// get is implemented differently for Firefox based on the OS.
func (f firefox) get(ctx context.Context, addr string) error {
	cmd := exec.CommandContext(ctx, f.path, "--profile", f.profileDirectory, "--headless", addr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute binary: %w", err)
	}
	return nil
}

// close is implemented differently for Firefox based on the OS.
func (f firefox) close() error {
	return os.RemoveAll(f.profileDirectory)
}
