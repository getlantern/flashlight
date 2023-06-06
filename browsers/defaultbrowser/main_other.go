//go:build !windows && !darwin
// +build !windows,!darwin

package main

import "github.com/getlantern/flashlight/v7/browsers"

func printBrowserDetails(b browsers.Browser) {}
