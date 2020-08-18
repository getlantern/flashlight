// Command defaultbrowser prints the name of the system's default web browser. It is difficult to
// write meaningful unit tests for something like this, but this utility at least lets us do some
// manual spot checking.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/getlantern/flashlight/browsers"
)

var timeout = flag.Duration("timeout", 5*time.Second, "")

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	start := time.Now()
	b, err := browsers.SystemDefault(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed:", err)
		os.Exit(1)
	}
	fmt.Printf("The default browser is %v.\nThis operation took %v\n", b, time.Now().Sub(start))

	printBrowserDetails(b)
}
