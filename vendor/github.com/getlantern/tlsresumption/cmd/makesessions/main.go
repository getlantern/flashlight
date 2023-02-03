// This program generates 1 or more client session states by handshaking with a TLS server and
// writes them to stdout in Base64 encoded form, with one session per line.
package main

import (
	"flag"
	"fmt"

	"github.com/getlantern/golog"
	"github.com/getlantern/tlsresumption"
)

var (
	log = golog.LoggerFor("makesessions")
)

func main() {
	num := flag.Int("num", 1, "number of client session states to create")
	flag.Parse()

	addr := flag.Arg(0)
	sessions, err := tlsresumption.MakeClientSessionStates(addr, *num)
	if err != nil && len(sessions) == 0 {
		log.Fatal(err)
	}

	for _, session := range sessions {
		fmt.Println(session)
	}
}
