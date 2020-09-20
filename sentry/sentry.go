package sentry

import (
	"regexp"
	"strings"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/osversion"
	sentrySDK "github.com/getsentry/sentry-go"
)

var (
	log                = golog.LoggerFor("flashlight.sentry")
	messageLinesRegex  = regexp.MustCompile(`\s*((\/\S*\/)\S+.\S+:\d+)`)
	goroutineRegex     = regexp.MustCompile(`goroutine \d+ \[.*\]:`)
	windowsPathRegex   = regexp.MustCompile(`[a-z|A-Z]:\\([A-Za-z_\-\s0-9\.]+\\)+`)
	unixPathRegex      = regexp.MustCompile(`\/([A-Za-z_\-\s0-9\.]+\/)+`)
	localhostPortRegex = regexp.MustCompile(`((127.0.0.1)|(localhost)):\d+`)
	cleanerRegexes     = []*regexp.Regexp{windowsPathRegex, unixPathRegex, localhostPortRegex}
)

type Opts struct {
	DSN             string
	MaxMessageChars int
}

func beforeSend(event *sentrySDK.Event, hint *sentrySDK.EventHint, sentryOpts Opts) *sentrySDK.Event {
	for i, exception := range event.Exception {
		event.Exception[i].Value = hidden.Clean(exception.Value)
	}
	event.Fingerprint = generateFingerprint(event)

	// sentry's sdk has a somewhat undocumented max message length
	// after which it seems it will silently drop/fail to send messages
	// https://github.com/getlantern/flashlight/pull/806
	event.Message = util.TrimStringAsBytes(event.Message, sentryOpts.MaxMessageChars)
	return event
}

func InitSentry(sentryOpts Opts) {
	sentrySDK.Init(sentrySDK.ClientOptions{
		Dsn:     sentryOpts.DSN,
		Release: common.Version,
		BeforeSend: func(event *sentrySDK.Event, hint *sentrySDK.EventHint) *sentrySDK.Event {
			return beforeSend(event, hint, sentryOpts)
		},
	})

	sentrySDK.ConfigureScope(func(scope *sentrySDK.Scope) {
		os_version, err := osversion.GetHumanReadable()
		if err != nil {
			log.Errorf("Unable to get os version: %v", err)
		} else {
			scope.SetTag("os_version", os_version)
		}
	})
}

func generateFingerprint(event *sentrySDK.Event) []string {
	// An attempt at keeping sentry from grouping distinct panics with the same top level message
	// see https://github.com/getlantern/lantern-internal/issues/3651
	// and https://docs.sentry.io/data-management/event-grouping/sdk-fingerprinting/?platform=go
	messageFingerprint := ""
	messageLines := strings.Split(event.Message, "\n")
	// always take the first line
	fingerprintLines := []string{messageLines[0]}
	// after the first line, consider the remainder of lines and only capture the file and line number
	// only capture the first seen goroutine for deterministic results
	seenGoroutine := false
	for i := 1; i < len(messageLines); i++ {
		line := messageLines[i]
		if goroutineRegex.MatchString(line) {
			if seenGoroutine {
				break
			}
			seenGoroutine = true
		}
		matches := messageLinesRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			fingerprintLines = append(fingerprintLines, matches[1])
		}
	}
	messageFingerprint = strings.Join(fingerprintLines, "\n")

	exceptionFingerprint := ""
	for _, exception := range event.Exception {
		// Sentry is already correctly separating exceptions by stack traces, but inexplicably not
		// by the exception.Value, so we have to add it here
		// https://github.com/getlantern/lantern-internal/issues/3666
		cleanedValue := exception.Value
		for _, re := range cleanerRegexes {
			cleanedValue = re.ReplaceAllString(cleanedValue, "")
		}
		exceptionFingerprint += cleanedValue
	}
	return []string{"{{ default }}", messageFingerprint, exceptionFingerprint}
}
