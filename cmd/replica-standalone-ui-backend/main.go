package main

import (
	"net/http"
	"os"

	"github.com/anacrolix/log"
	"github.com/getlantern/flashlight/desktop/replica"
)

func main() {
	code := mainCode()
	if code != 0 {
		os.Exit(code)
	}
}

func mainCode() int {
	handler, exitFunc, err := replica.NewHttpHandler()
	if err != nil {
		log.Print("error creating replica http server: %v", err)
		return 1
	}
	defer exitFunc()
	panic(http.ListenAndServe("", handler))
}
