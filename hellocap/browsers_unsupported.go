// +build !windows,!darwin darwin,ios

package hellocap

import "errors"

func defaultBrowser() (browser, error) {
	return nil, errors.New("unsupported platform")
}
