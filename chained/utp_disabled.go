// +build android ios linux

package chained

import "errors"

func enableUTP(wrapped proxyImpl, addr string) (proxyImpl, error) {
	return nil, errors.New("UTP is not supported on Android, iOS or Linux")
}
