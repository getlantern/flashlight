package main

import (
	"fmt"

	"github.com/getlantern/flashlight/browsers"
)

func printBrowserDetails(b browsers.Browser) {
	p, err := b.Executable()
	if err != nil {
		fmt.Println("failed to obtain browser executable path:", err)
	} else {
		fmt.Println("executable path:", p)
	}
}
