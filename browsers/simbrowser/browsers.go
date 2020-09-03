package simbrowser

import (
	"time"

	"github.com/getlantern/flashlight/deterministic"
	tls "github.com/refraction-networking/utls"
)

type browserBehavior struct {
	name                  string
	sessionTicketLifetime time.Duration
	clientHelloID         tls.ClientHelloID
}

func (bb browserBehavior) Name() string                         { return bb.name }
func (bb browserBehavior) SessionTicketLifetime() time.Duration { return bb.sessionTicketLifetime }
func (bb browserBehavior) ClientHelloID() tls.ClientHelloID     { return bb.clientHelloID }

type browserChoice struct {
	browserBehavior
	marketShare float64
}

// Implements the deterministic.WeightedChoice interface.
func (bc browserChoice) Weight() int { return int(bc.marketShare * 100) }

var (
	// Session ticket lifetime data can be found here:
	// https://github.com/getlantern/lantern-internal/issues/3315#issue-560602994

	chrome     = browserBehavior{"Chrome", 30 * time.Minute, tls.HelloChrome_Auto}
	safari     = browserBehavior{"Safari", 24 * time.Hour, tls.HelloSafari_Auto}
	firefox    = browserBehavior{"Firefox", 24 * time.Hour, tls.HelloFirefox_Auto}
	edge       = browserBehavior{"Edge", 10 * time.Hour, tls.HelloEdge_Auto}
	explorer   = browserBehavior{"Internet Explorer", 600 * time.Minute, tls.HelloExplorer_11}
	threeSixty = browserBehavior{"360 Secure Browser", 9 * time.Hour, tls.Hello360_Auto}
	qq         = browserBehavior{"QQ Browser", 30 * time.Minute, tls.HelloQQ_Auto}

	// https://gs.statcounter.com/browser-market-share#monthly-201910-201910-bar
	globalBrowserChoices = []deterministic.WeightedChoice{
		browserChoice{chrome, 0.65},
		browserChoice{safari, 0.17},
		// Using utls.HelloFirefox_65 results in an error, so we're disabling it for now.
		// Possibly related to https://github.com/getlantern/lantern-internal/issues/3850
		// browserChoice{firefox, 0.04},
		browserChoice{edge, 0.04},
	}

	browserChoicesByCountry = map[string][]deterministic.WeightedChoice{
		// https://github.com/getlantern/lantern-internal/issues/3315#issuecomment-589253390
		"CN": {
			browserChoice{edge, 0.36},
			browserChoice{threeSixty, 0.26},
			browserChoice{qq, 0.10},
			// Using utls.HelloFirefox_65 results in an error, so we're disabling it for now.
			// Possibly related to https://github.com/getlantern/lantern-internal/issues/3850
			// browserChoice{firefox, 0.03},
		},
	}
)
