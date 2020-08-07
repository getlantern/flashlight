package hellocap

import (
	"errors"
	"fmt"
	"os"

	"github.com/mitchellh/go-ps"
)

type errNoSuchProcess int

func (e errNoSuchProcess) Error() string {
	return fmt.Sprintf("could not find process with PID '%d'", e)
}

type process struct {
	ps.Process
	osHandle *os.Process

	children []process
}

// processTree returns a tree of processes and their children, rooted at the current process.
func processTree() (root *process, err error) {
	allProcessesSnapshot, err := ps.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to look up snapshot of current processes: %w", err)
	}
	root, err = ptHelper(os.Getpid(), allProcessesSnapshot)
	if _, ok := err.(errNoSuchProcess); ok {
		return nil, errors.New("failed to look up self")
	}
	return
}

func ptHelper(rootPID int, allProcessesSnapshot []ps.Process) (*process, error) {
	rootP, err := ps.FindProcess(rootPID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up process by PID %d", rootPID)
	}
	if rootP == nil {
		return nil, errNoSuchProcess(rootPID)
	}
	rootHandle, err := os.FindProcess(rootPID)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain process handle: %w", err)
	}
	root := process{rootP, rootHandle, []process{}}
	for _, p := range allProcessesSnapshot {
		if p.PPid() == rootPID {
			childTree, err := ptHelper(p.Pid(), allProcessesSnapshot)
			if _, ok := err.(errNoSuchProcess); ok {
				// The child may have died since the snapshot was taken, just exclude it.
				continue
			} else if err != nil {
				return nil, fmt.Errorf(
					"failed to look up child tree started by %s: %w", rootP.Executable(), err)
			}
			root.children = append(root.children, *childTree)
		}
	}
	return &root, nil
}

// kill the process and all of its descendents via postorder traversal.
func (p *process) kill() error {
	for _, child := range p.children {
		if err := child.kill(); err != nil {
			return fmt.Errorf(
				"failed to kill process tree started by %s: %w", child.Executable(), err)
		}
	}
	return p.osHandle.Kill()
}
