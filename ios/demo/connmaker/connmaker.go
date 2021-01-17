// connmaker is a utility for creating lots of TCP connections without closing them, to help stress the memory on the ios demo app
package main

import (
	"net"
)

func main() {
	for i := 0; i < 1000; i++ {
		net.Dial("tcp", "104.131.174.43:443")
	}
}
