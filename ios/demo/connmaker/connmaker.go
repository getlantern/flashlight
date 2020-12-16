package main

import (
	"net"
)

func main() {
	for i := 0; i < 1000; i++ {
		net.Dial("tcp", "104.131.174.43:443")
	}
}
