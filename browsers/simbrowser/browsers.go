package simbrowser

import (
	"fmt"
	"sync"
	"time"

	"github.com/getlantern/flashlight/browsers"
	"github.com/getlantern/flashlight/deterministic"
	tls "github.com/refraction-networking/utls"
)

// CountryCode is a 2-letter ISO country code.
type CountryCode [2]rune

var globally CountryCode = [2]rune{'*', '*'}

// MarketShare is a value between 0 and 1 representing a fraction of the global market.
type MarketShare float64

// Platform describes an operating system and should match a GOOS value. Run 'go tool dist list' to
// see available options. An exception is made for iOS, which should be specifed with the value
// 'ios'.
type Platform string

// MarketShareData encapsulates market share information for a given combination of browser and
// platform.
type MarketShareData struct {
	Browser             browsers.Browser
	Platform            Platform
	GlobalMarketShare   MarketShare
	RegionalMarketShare map[CountryCode]MarketShare
}

var (
	marketShareData map[Platform]map[CountryCode]map[browsers.Browser]MarketShare
	marketShareLock sync.Mutex
)

// SetMarketShareData sets the data used by this package to assign browsers to users.
func SetMarketShareData(data []MarketShareData) error {
	msData := map[Platform]map[CountryCode]map[browsers.Browser]MarketShare{}
	for _, dataPoint := range data {
		msData[dataPoint.Platform][globally][dataPoint.Browser] = dataPoint.GlobalMarketShare
	}
}

type browserBehavior struct {
	name                  string
	sessionTicketLifetime time.Duration
	clientHelloID         tls.ClientHelloID
}

// TODO: this function needs to be platform specific
func getBrowserBehavior(b browsers.Browser) (*browserBehavior, error) {
	switch b {
	case browsers.Chrome:
	case browsers.Safari:
	case browsers.Firefox:
	case browsers.Edge:
	default:
		return nil, fmt.Errorf("unsupported browser %v", b)
	}
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
