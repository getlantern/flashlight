package main

import (
	"flag"
	"fmt"
	"net"

	tls "github.com/refraction-networking/utls"
)

var addr = flag.String("addr", "localhost:2000", "")

func main() {
	tcpConn, err := net.Dial("tcp", *addr)
	if err != nil {
		panic(err)
	}
	conn := tls.UClient(tcpConn, &tls.Config{InsecureSkipVerify: true}, tls.HelloRandomized)
	defer conn.Close()
	if err := conn.Handshake(); err != nil {
		panic(err)
	}
	fmt.Println("handshake complete")
}
