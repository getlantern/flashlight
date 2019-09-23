package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/getlantern/flashlight/diagnostics"
)

const (
	maxCapturePackets = 1000000
	maxSavePackets    = 1000000
)

var address = flag.String("address", "167.71.87.46:443", "address to capture for (ip:port)")

func fail(a ...interface{}) {
	fmt.Fprintln(os.Stderr, a...)
	os.Exit(1)
}

func main() {
	tl := diagnostics.NewTrafficLog(maxCapturePackets, maxSavePackets)
	if err := tl.UpdateAddresses([]string{*address}); err != nil {
		fail("failed to start traffic log:", err)
	}
	defer tl.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan
}
