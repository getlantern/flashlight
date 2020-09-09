// Package browsers provides utilities centered around web browsers.
package browsers

import (
	"errors"
	"fmt"
	"os/exec"
)

// ErrUnsupportedAction is returned when a function is not supported for a given browser.
var ErrUnsupportedAction = errors.New("this action is not supported for this web browser")

// Browser represents a specific web browser.
type Browser int

func wrapExecError(msg string, err error) error {
	var execErr *exec.ExitError
	if errors.As(err, &execErr) {
		return fmt.Errorf("%w: %s", err, string(execErr.Stderr))
	}
	return err
}
