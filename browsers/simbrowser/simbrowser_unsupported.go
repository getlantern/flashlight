// +build !darwin,!windows darwin,ios

package simbrowser

import (
	"context"
	"errors"
)

func mimicDefaultBrowser(_ context.Context) (*Browser, error) {
	return nil, errors.New("unsupported platform")
}
