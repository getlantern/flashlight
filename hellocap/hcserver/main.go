package main

import (
	"fmt"

	"github.com/getlantern/flashlight/hellocap"
)

func main() {
	serverErrors := make(chan error)
	addr, close, err := hellocap.StartServer(func(hello []byte, err error) {
		if err != nil {
			fmt.Println("capture error:", err)
		} else {
			fmt.Println("captured", len(hello), "byte hello")
		}
	}, serverErrors)
	if err != nil {
		panic(err)
	}
	defer close()
	fmt.Println("listening on", addr)

	if err := <-serverErrors; err != nil {
		panic(err)
	}
}
