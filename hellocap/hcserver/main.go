package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/getlantern/flashlight/hellocap"
)

// TODO: could create a genspec library and print specs here - or maybe an option on genspec to start a server?

func main() {
	s, err := hellocap.NewServer(func(hello []byte, err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "error capturing hello:", err)
		} else {
			fmt.Println(base64.StdEncoding.EncodeToString(hello))
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start server:", err)
		os.Exit(1)
	}
	defer s.Close()

	fmt.Fprintf(os.Stderr, "listening on https://%v\n", s.Addr())
	if err := s.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
