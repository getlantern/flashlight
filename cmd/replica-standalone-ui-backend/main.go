package main

import (
	"net/http"
	"os"

	"github.com/anacrolix/log"
	"github.com/anacrolix/tagflag"
	"github.com/getlantern/appdir"
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
	handler, err := desktopReplica.NewHTTPHandler(
		appdir.General("ReplicaStandalone"),
		uc,
		replica.Client{
			Storage:  replica.S3Storage{},
			Endpoint: flags.Endpoint,
		},
		// TODO: make this configurable for Iran
		replica.Client{
			Storage: replica.S3Storage{},
			Endpoint: replica.Endpoint{
				StorageProvider: "s3",
				BucketName:      "replica-metadata",
				Region:          "ap-southeast-1",
			}},
&analytics.NullSession{},
		desktopReplica.DefaultNewHttpHandlerOpts(),
	)
	if err != nil {
		log.Printf("error creating replica http server: %v", err)
		return 1
	}
	defer handler.Close()
	panic(http.ListenAndServe("", handler))
}
