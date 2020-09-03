package main

import (
	"context"
	"fmt"

	"github.com/getlantern/flashlight/browsers"
)

func printBrowserDetails(b browsers.Browser) {
	bundle, err := b.AppBundle(context.Background())
	if err != nil {
		fmt.Println("failed to obtain application bundle:", err)
	} else {
		fmt.Println("application bundle:", bundle)
	}
}
