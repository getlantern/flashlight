package sentry

import (
	"time"

	"github.com/getlantern/flashlight/v7/util"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/osversion"
	"github.com/getsentry/sentry-go"
)

var log = golog.LoggerFor("flashlight.sentry")

const (
	// Sentry Configurations
	SentryTimeout         = time.Second * 30
	SentryMaxMessageChars = 8000
	// SentryDSN is Sentry's project ID thing
	SentryDSN = "https://f65aa492b9524df79b05333a0b0924c5@o75725.ingest.us.sentry.io/2222244"
)

func beforeSend(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	for i, exception := range event.Exception {
		event.Exception[i].Value = hidden.Clean(exception.Value)
	}

	// sentry's sdk has a somewhat undocumented max message length
	// after which it seems it will silently drop/fail to send messages
	// https://github.com/getlantern/flashlight/pull/806
	event.Message = util.TrimStringAsBytes(event.Message, SentryMaxMessageChars)
	return event
}

func InitSentry(libraryVersion string) {
	log.Debugf("Initializing Sentry with library version: %s", libraryVersion)
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              SentryDSN,
		Release:          libraryVersion,
		AttachStacktrace: true,
		BeforeSend:       beforeSend,
	})
	if err != nil {
		log.Errorf("Failed to initialize Sentry: %v", err)
		return
	}

	sentry.ConfigureScope(func(scope *sentry.Scope) {
		os, err := osversion.GetHumanReadable()
		if err != nil {
			log.Errorf("Unable to get os version: %v", err)
		} else {
			scope.SetTag("os_version", os)
		}
	})
	if result := sentry.Flush(SentryTimeout); !result {
		log.Error("Flushing to Sentry timed out")
	} else {
		log.Debug("Flushed to Sentry")
	}
	log.Debug("Sentry initialized")
}

func PanicListener(msg string) {
	log.Errorf("Panic in kindling: %v", msg)
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
	})

	sentry.CaptureMessage(msg)
	if result := sentry.Flush(SentryTimeout); !result {
		log.Error("Flushing to Sentry timed out")
	}
}
