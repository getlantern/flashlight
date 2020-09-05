package main

import (
	"net/http"
	"os"

	"github.com/anacrolix/log"
	"github.com/anacrolix/tagflag"
	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/common"
	desktopReplica "github.com/getlantern/flashlight/desktop/replica"
	"github.com/getlantern/replica"
)

type flags struct {
	replica.Endpoint
}

func main() {
	flags := flags{
		Endpoint: replica.DefaultEndpoint,
	}
	tagflag.Parse(&flags)
	code := mainCode(flags)
	if code != 0 {
		os.Exit(code)
	}
}

func mainCode(flags flags) int {
	uc := common.NewUserConfigData("replica-standalone", 0, "replica-standalone-token", nil, "en-US")
	handler, exitFunc, err := desktopReplica.NewHTTPHandler(
		uc,
		&replica.Client{
			HttpClient: http.DefaultClient,
			Endpoint:   flags.Endpoint,
		},
		&analytics.NullSession{},
	)
	if err != nil {
		log.Printf("error creating replica http server: %v", err)
		return 1
	}
	defer exitFunc()
	panic(http.ListenAndServe("", handler))
}
