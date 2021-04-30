// +build !windows,!darwin darwin,iosapp

package hellocap

import (
	"context"
	"errors"
)

func defaultBrowser(ctx context.Context) (browser, error) {
	return nil, errors.New("unsupported platform")
}
