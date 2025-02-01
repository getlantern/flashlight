package common

import (
	"time"

	"github.com/getlantern/golog"
)

const (
	// UserConfigURL is the URL for fetching the per user proxy config.
	UserConfigURL = "https://df.iantem.io/api/v1/config"

	// Sentry Configurations
	SentryTimeout         = time.Second * 30
	SentryMaxMessageChars = 8000

	// UpdateServerURL is the URL of the update server. Different applications
	// hit the server on separate paths "/update/<AppName>".
	UpdateServerURL = "https://update.getlantern.org/"
)

var (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
)

var (
	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.flashlightproxy.com/global.yaml.gz"

	ProAPIHost = "api.getiantem.org"

	log = golog.LoggerFor("flashlight.common")

	// Set by the linker using -ldflags in the project's Makefile.
	// Defaults to 'production' so as not to mistakingly push development work
	// to a production environment
	Environment = "production"
)
