//go:build !iosapp
// +build !iosapp

package common

import "runtime"

const Platform = runtime.GOOS
