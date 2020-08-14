package hellocap

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type chrome struct {
	path string
}

func (c chrome) name() string { return "Google Chrome" }
func (c chrome) close() error { return nil }

func (c chrome) get(ctx context.Context, addr string) error {
	// The --disable-gpu flag is necessary to run headless Chrome on Windows:
	// https://bugs.chromium.org/p/chromium/issues/detail?id=737678
	if err := exec.CommandContext(ctx, c.path, "--headless", "--disable-gpu", addr).Run(); err != nil {
		// The Chrome binary does not appear to ever exit non-zero, so we don't need to worry about
		// catching and ignoring errors due to things like certificate validity checks.
		return fmt.Errorf("failed to execute binary: %w", err)
	}
	return nil
}

type edgeChromium struct {
	path string
}

func (ec edgeChromium) name() string { return "Microsoft Edge" }
func (ec edgeChromium) close() error { return nil }

func (ec edgeChromium) get(ctx context.Context, addr string) error {
	if err := exec.CommandContext(ctx, ec.path, "--headless", addr).Run(); err != nil {
		return fmt.Errorf("failed to execute binary: %w", err)
	}
	return nil
}

// Firefox only allows one active instance per profile. We create a new profile in a
// temporary directory and clean it up when we're done.
func newFirefoxProfileDirectory() (string, error) {
	tmpDir, err := ioutil.TempDir("", "lantern.test-firefox-profile")
	if err != nil {
		return "", fmt.Errorf("failed to set up temporary directory: %w", err)
	}

	timestampData := fmt.Sprintf(`{
"created": %d,
"firstUse": null
}`, time.Now().Unix()*1000)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "times.json"), []byte(timestampData), 0644)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to write timestamp file: %w", err)
	}
	return tmpDir, nil
}
