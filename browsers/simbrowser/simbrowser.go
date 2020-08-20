// Package simbrowser provides facilities for simulating aspects of web browsers.
package simbrowser

import (
	"context"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/deterministic"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/golog"
	tls "github.com/refraction-networking/utls"
)

// This timeout should be longer than anything a caller might pass to us. We use this to avoid
// goroutine leaks.
const longTimeout = 10 * time.Minute

var log = golog.LoggerFor("simbrowser")

// Browser represents a specific web browser, e.g. Chrome or Firefox.
type Browser interface {
	Name() string
	SessionTicketLifetime() time.Duration
	ClientHelloSpec() tls.ClientHelloSpec
}

// ChooseForUser chooses a web browser for the input user. This choice is deterministic for a given
// user ID, and the distribution of choices across user IDs will reflect the market share of the top
// 4 browsers.
//
// If necessary, we use region-specific market share figures. This is based on the client's
// geolocation and thus this function may block for a period while geolocation is determined. If the
// context expires before the client's geolocation can be determined, global market shares will be
// used.
func ChooseForUser(ctx context.Context, uc common.UserConfig) Browser {
	geolookupTimeout := longTimeout
	if deadline, ok := ctx.Deadline(); ok {
		// A timeout of 0 tells geolookup to return immediately.
		geolookupTimeout = max(deadline.Sub(time.Now()), 0)
	}

	countryCodeC := make(chan string, 1)
	go func() {
		if countryCode := geolookup.GetCountry(geolookupTimeout); countryCode != "" {
			countryCodeC <- countryCode
		}
	}()

	choices := globalBrowserChoices
	select {
	case countryCode := <-countryCodeC:
		if countryChoices, ok := browserChoicesByCountry[countryCode]; ok {
			choices = countryChoices
		}
	case <-ctx.Done():
		log.Debug("context expired retrieving country code, falling back to global browser choices")
	}

	choice := deterministic.MakeWeightedChoice(uc.GetUserID(), choices)
	return choice.(browserChoice).browserBehavior
}

func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
