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
type Browser struct {
	Type                  BrowserType
	SessionTicketLifetime time.Duration
	ClientHelloID         tls.ClientHelloID
}

var (
	// Session ticket lifetime data can be found here:
	// https://github.com/getlantern/lantern-internal/issues/3315#issue-560602994

	chrome     = Browser{Chrome, 30 * time.Minute, tls.HelloChrome_Auto}
	firefox    = Browser{Firefox, 24 * time.Hour, tls.HelloFirefox_Auto}
	edge       = Browser{Edge, 10 * time.Hour, tls.HelloEdge_Auto}
	explorer   = Browser{InternetExplorer, 600 * time.Minute, tls.HelloExplorer_11}
	threeSixty = Browser{ThreeSixtySecureBrowser, 9 * time.Hour, tls.Hello360_Auto}
	qq         = Browser{QQBrowser, 30 * time.Minute, tls.HelloQQ_Auto}
	safari     = Browser{Safari, 24 * time.Hour, tls.HelloSafari_Auto}
)

// ChooseForUser chooses a web browser for the input user. If it is possible to determine the
// system's default web browser, a corresponding Browser instance will be returned. If not, this
// function chooses a browser to mimic. This choice is deterministic for a given user ID, and the
// distribution of choices across user IDs will reflect the market share of the top 4 browsers.
//
// If necessary, we use region-specific market share figures. This is based on the client's
// geolocation and thus this function may block for a period while geolocation is determined. If the
// context expires before the client's geolocation can be determined, global market shares will be
// used.
func ChooseForUser(ctx context.Context, uc common.UserConfig) Browser {
	type browserResult struct {
		b   *Browser
		err error
	}

	geolookupTimeout := longTimeout
	if deadline, ok := ctx.Deadline(); ok {
		// A timeout of 0 tells geolookup.DefaultInstance to return immediately.
		geolookupTimeout = max(deadline.Sub(time.Now()), 0)
	}

	defaultBrowserC := make(chan browserResult, 1)
	go func() {
		b, err := mimicDefaultBrowser(ctx)
		defaultBrowserC <- browserResult{b, err}
	}()

	countryCodeC := make(chan CountryCode, 1)
	go func() {
		if cc := geolookup.DefaultInstance.GetCountry(geolookupTimeout); cc != "" {
			countryCodeC <- CountryCode(cc)
		}
	}()

	select {
	case res := <-defaultBrowserC:
		if res.err == nil {
			return *res.b
		}
		log.Debugf("failed to mimic default browser: %v; falling back to weighted choice", res.err)
	case <-ctx.Done():
		log.Debugf("context expired determining default browser, falling back to weighted choice")
	}

	marketShareLock.RLock()
	choices := marketShareData[globally]
	select {
	case countryCode := <-countryCodeC:
		if countryChoices, ok := marketShareData[countryCode]; ok {
			choices = countryChoices
		}
	default:
		log.Debug("context expired retrieving country code, falling back to global browser choices")
	}
	marketShareLock.RUnlock()

	weightedChoices := []deterministic.WeightedChoice{}
	for _, c := range choices {
		if c.Browser.Type == Firefox {
			// Using utls.HelloFirefox_65 results in an error, so we're disabling it for now.
			// Possibly related to https://github.com/getlantern/lantern-internal/issues/3850
			continue
		}
		weightedChoices = append(weightedChoices, c)
	}

	choice := deterministic.MakeWeightedChoice(uc.GetUserID(), weightedChoices)
	return choice.(browserChoice).Browser
}

func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
