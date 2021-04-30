// +build !windows,!darwin darwin,iosapp

package browsers

import (
	"context"
	"errors"
)

// SystemDefault is not supported on this platform.
func SystemDefault(_ context.Context) (Browser, error) {
	return 0, errors.New("unsupported platform")
}
