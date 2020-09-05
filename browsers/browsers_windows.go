package browsers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getlantern/flashlight/ops"
	"golang.org/x/sys/windows/registry"
)

// ErrorUnknownProgramID means that an unknown progran ID was encountered. However, this program ID
// may still be useful.
type ErrorUnknownProgramID string

func (err ErrorUnknownProgramID) Error() string {
	return fmt.Sprintf("unknown program ID '%s'", string(err))
}

// ProgramID provides the unrecognized program ID underlying this error. This function is provided
// for convenience and readability; it is equivalent to string(err).
func (err ErrorUnknownProgramID) ProgramID() string {
	return string(err)
}

// Possible Browsers.
const (
	Unknown Browser = iota
	Chrome
	Firefox
	Edge
	InternetExplorer
	ThreeSixtySecureBrowser
	QQBrowser

	// EdgeLegacy is the older, HTML-based version of Microsoft Edge.
	// https://support.microsoft.com/en-us/help/4026494/microsoft-edge-difference-between-legacy
	EdgeLegacy
)

func (b Browser) String() string {
	switch b {
	case Chrome:
		return "Google Chrome"
	case Firefox:
		return "Mozilla Firefox"
	case Edge:
		return "Microsoft Edge"
	case EdgeLegacy:
		return "Microsoft Edge Legacy"
	case InternetExplorer:
		return "Microsoft Internet Explorer"
	case ThreeSixtySecureBrowser:
		return "Qihoo 360 Secure Browser"
	case QQBrowser:
		return "Tencent QQ Browser"
	default:
		return "Unknown"
	}
}

func (b Browser) programID() (string, error) {
	switch b {
	case InternetExplorer:
		return "htmlfile", nil
	case ThreeSixtySecureBrowser:
		return "360BrowserURL", nil
	case QQBrowser:
		return "QQBrowser.File", nil
	case Chrome:
		return "ChromeHTML", nil
	case Edge:
		return "MSEdgeHTM", nil
	case Firefox:
		// Firefox appears to use a non-constant program ID.
		fallthrough
	default:
		return "", fmt.Errorf("no known program ID for '%v'", b)
	}
}

var execPathRegexp = regexp.MustCompile(`"([^"]*)".*`)

// Executable returns the absolute path to the executable file for this browser.
func (b Browser) Executable() (string, error) {
	if b == Firefox {
		// The program ID for Firefox is unpredictable (as far as we know anyway), so we can't use
		// it to reliably find the executable. We just look in expected locations instead.
		for _, execPath := range []string{
			filepath.Join(`C:\`, "Program Files", "Mozilla Firefox", "firefox.exe"),
			filepath.Join(`C:\`, "Program Files (x86)", "Mozilla Firefox", "firefox.exe"),
		} {
			_, err := os.Stat(execPath)
			if err == nil {
				return execPath, nil
			}
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to stat expected path for executable: %w", err)
			}
		}
		return "", errors.New("could not find executable in expected locations")
	}

	progID, err := b.programID()
	if err != nil {
		return "", fmt.Errorf("failed to obtain browser's program ID: %w", err)
	}
	appExec, err := registry.OpenKey(
		registry.CLASSES_ROOT, fmt.Sprintf(`%s\Shell\open\command`, progID), registry.READ)
	if err != nil {
		return "", fmt.Errorf("failed to read browser executable info from registry: %w", err)
	}
	regEntry, _, err := appExec.GetStringValue("")
	if err != nil {
		return "", fmt.Errorf("failed to read path to browser executable from registry: %w", err)
	}
	matches := execPathRegexp.FindStringSubmatch(regEntry)
	if len(matches) <= 1 {
		return "", errors.New("unexpected executable path structure")
	}
	return matches[1], nil
}

// SystemDefault returns the default web browser. Specifically, this is the default handler for HTML
// files. May return ErrorUnknownProgramID.
func SystemDefault(ctx context.Context) (Browser, error) {
	op := ops.Begin("get_default_browser")
	op.Set("os", "windows")
	b, err := systemDefault(ctx)
	op.FailIf(err)
	op.End()
	return b, err
}

func systemDefault(ctx context.Context) (Browser, error) {
	// TODO: test on Windows < 10 ? Probably just Windows 7 is good
	// may need https://stackoverflow.com/a/2178637 for older versions of Windows

	// https://stackoverflow.com/a/12444963
	userChoice, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.html\UserChoice`,
		registry.READ,
	)
	if err != nil {
		return Unknown, fmt.Errorf("failed to read default browser from registry: %w", err)
	}
	progID, _, err := userChoice.GetStringValue(`ProgId`)
	if err != nil {
		return Unknown, fmt.Errorf("failed to read browser program ID from registry: %w", err)
	}

	if strings.Contains(progID, "Firefox") {
		return Firefox, nil
	}

	for _, b := range []Browser{
		InternetExplorer, ThreeSixtySecureBrowser, QQBrowser, Chrome, Edge,
	} {
		bProgID, err := b.programID()
		if err != nil {
			return Unknown, fmt.Errorf("unexpected error getting program ID for %v: %w", b, err)
		}
		if progID == bProgID {
			return b, nil
		}
	}
	return Unknown, ErrorUnknownProgramID(progID)
}
