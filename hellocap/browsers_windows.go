package hellocap

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mitchellh/go-ps"

	"github.com/getlantern/flashlight/browsers"
)

func defaultBrowser(ctx context.Context) (browser, error) {
	_browser, err := browsers.SystemDefault(ctx)
	if err != nil {
		return nil, err
	}

	if _browser == browsers.Firefox {
		f, err := newFirefoxInstance()
		if err != nil {
			return nil, fmt.Errorf("failed to create new Firefox instance: %w", err)
		}
		return f, nil
	}

	execPath, err := _browser.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain executable for '%v': %w", _browser, err)
	}

	switch _browser {
	case browsers.Edge:
		return edgeChromium{execPath}, nil
	case browsers.Chrome:
		return chrome{execPath}, nil
	default:
		return nil, fmt.Errorf("unsupported browser '%v'", _browser)
	}
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
