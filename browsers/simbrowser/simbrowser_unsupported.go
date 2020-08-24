// +build !darwin,!windows darwin,ios

package simbrowser

import (
	"context"
	"errors"
)

// mimicDefaultBrowser is unsupported on this platform.
func mimicDefaultBrowser(_ context.Context) (Browser, error) {
	return nil, errors.New("unsupported platform")
}
