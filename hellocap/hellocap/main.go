// Command hellocap captures a sample ClientHello from the default browser and prints it to stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/getlantern/flashlight/hellocap"
)

var timeout = flag.Duration("timeout", 10*time.Second, "")

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	hello, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(string(hello))
}
