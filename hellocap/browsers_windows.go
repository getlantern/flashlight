package hellocap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/mitchellh/go-ps"
	"golang.org/x/sys/windows/registry"
)

func defaultBrowser(ctx context.Context) (browser, error) {
	// TODO: test on Windows < 10 ?
	// may need https://stackoverflow.com/a/2178637 for older versions of Windows

	// https://stackoverflow.com/a/12444963
	userChoice, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.html\UserChoice`, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser from registry: %w", err)
	}
	progID, _, err := userChoice.GetStringValue(`ProgId`)
	if err != nil {
		return nil, fmt.Errorf("failed to read browser program ID from registry: %w", err)
	}
	fmt.Println("progID:", progID)

	var appName string
	switch {
	case progID == "htmlfile":
		appName = "Microsoft Internet Explorer"
	case strings.Contains(progID, "Firefox"):
		appName = "Mozilla Firefox"
	case progID == "360BrowserURL":
		return nil, errors.New("unsupported browser 'Qihoo 360 Secure Browser'")
	case progID == "QQBrowser.File":
		return nil, errors.New("unsupported browser 'Tencent QQBrowser'")
	default:
		application, err := registry.OpenKey(registry.CLASSES_ROOT, fmt.Sprintf(`%s\Application`, progID), registry.READ)
		if err != nil {
			return nil, fmt.Errorf("failed to read default browser application info from registry: %w", err)
		}
		appName, _, err = application.GetStringValue(`ApplicationName`)
		if err != nil {
			return nil, fmt.Errorf("failed to read default browser name from registry: %w", err)
		}
	}
	fmt.Println("appName:", appName)

	appExec, err := registry.OpenKey(registry.CLASSES_ROOT, fmt.Sprintf(`%s\Shell\open\command`, progID), registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser executable info from registry: %w", err)
	}
	execPath, _, err := appExec.GetStringValue("")
	if err != nil {
		return nil, fmt.Errorf("failed to read path to default browser executable from registry: %w", err)
	}
	fmt.Println("execPath:", execPath)

	switch appName {
	case "Microsoft Edge":
		// TODO: detect difference between Edge and EdgeHTML
		fmt.Println("default browser is Edge")
		execPath, err := execPathFromRegistryEntry(execPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse executable path for Edge (Chromium): %w", err)
		}
		return edgeChromium{execPath}, nil

	case "Microsoft Internet Explorer":
		// TODO: implement me!
		return nil, errors.New("unsupported browser - Internet Explorer")

	case "Google Chrome":
		fmt.Println("default browser is Chrome")
		execPath, err := execPathFromRegistryEntry(execPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse executable path for Chrome: %w", err)
		}
		return chrome{execPath}, nil

	case "Mozilla Firefox":
		fmt.Println("default browser is Firefox")
		f, err := newFirefoxInstance()
		if err != nil {
			return nil, fmt.Errorf("failed to create new Firefox instance: %w", err)
		}
		return f, nil

	default:
		return nil, fmt.Errorf("unsupported browser '%s'", appName)
	}
}

var execPathRegexp = regexp.MustCompile(`"(.*)".*".*"`)

func execPathFromRegistryEntry(regEntry string) (string, error) {
	matches := execPathRegexp.FindStringSubmatch(regEntry)
	if len(matches) <= 1 {
		return "", errors.New("unexpected executable path structure")
	}
	fmt.Printf("using path '%s'\n", matches[1])
	return matches[1], nil
}

// EdgeHTML or Microsoft Edge Legacy is the older, HTML-based version of the Edge browser.
// https://support.microsoft.com/en-us/help/4026494/microsoft-edge-difference-between-legacy
type edgeHTML struct{}

func (eh edgeHTML) name() string { return "Microsoft Edge - HTML" }
func (eh edgeHTML) close() error { return nil }

func (eh edgeHTML) get(ctx context.Context, addr string) error {
	// TODO: implement me!
	return errors.New("edge HTML is not supported")
}

type firefox struct {
	profileDirectory string
	cmdPIDs          []int
}

func newFirefoxInstance() (*firefox, error) {
	pDir, err := newFirefoxProfileDirectory()
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary Firefox profile: %w", err)
	}
	fmt.Println("using profile in", pDir)
	return &firefox{pDir, []int{}}, nil
}

func (f *firefox) name() string { return "Mozilla Firefox" }

// get is implemented differently for Firefox based on the OS.
func (f *firefox) get(ctx context.Context, addr string) error {
	cmd := exec.CommandContext(
		ctx, "cmd", "/C", "start", "firefox", "--profile", f.profileDirectory, "-headless", addr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute binary: %w", err)
	}
	f.cmdPIDs = append(f.cmdPIDs, cmd.Process.Pid)
	return nil
}

// close is implemented differently for Firefox based on the OS.
func (f *firefox) close() error {
	if err := f.killChildProcesses(); err != nil {
		os.RemoveAll(f.profileDirectory)
		return fmt.Errorf("failed to kill spawned firefox processes: %w", err)
	}
	return os.RemoveAll(f.profileDirectory)
}

// On Windows, running Firefox in headless mode with the start command results in an orphaned tree
// of processes. This function cleans up any such trees.
func (f *firefox) killChildProcesses() error {
	if len(f.cmdPIDs) == 0 {
		return nil
	}

	fmt.Println("killing child Firefox processes")
	allProcs, err := ps.Processes()
	if err != nil {
		return fmt.Errorf("failed to obtain process snapshot: %w", err)
	}
	errs := []error{}
	for _, ppid := range f.cmdPIDs {
		for _, p := range allProcs {
			if p.PPid() != ppid {
				continue
			}
			pTree, err := processTree(p.Pid(), allProcs)
			if err != nil {
				errs = append(errs, fmt.Errorf(
					"failed to obtain process tree for process with executable '%s': %v",
					p.Executable(), err,
				))
				continue
			}
			fmt.Printf("killing process tree with executable '%s'\n", p.Executable())
			if err := pTree.kill(); err != nil {
				errs = append(errs, fmt.Errorf(
					"failed to kill process tree for executable '%s': %v",
					p.Executable(), err,
				))
			}
		}
	}
	f.cmdPIDs = []int{}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("%d errors; first: %w", len(errs), errs[0])
	}
}
