// Command hellocap captures a sample ClientHello from the default browser and prints it to stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/getlantern/flashlight/browsers"
	"github.com/getlantern/flashlight/hellocap"
)

var timeout = flag.Duration("timeout", 10*time.Second, "")

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	b, err := browsers.SystemDefault(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to obtain default browser:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "Default browser identified as", b)

	hello, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(hello))
}
