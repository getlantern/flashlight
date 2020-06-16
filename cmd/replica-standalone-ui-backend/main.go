package main

import (
	"net/http"
	"os"

	"github.com/getlantern/flashlight/common"
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
	uc := common.NewUserConfigData("replica-standalone", 0, "replica-standalone-token", nil, "en-US")
	handler, exitFunc, err := replica.NewHTTPHandler(uc)
	if err != nil {
		log.Printf("error creating replica http server: %v", err)
		return 1
	}
	defer exitFunc()
	panic(http.ListenAndServe("", handler))
}
